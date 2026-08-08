package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ADR-0005 D1. Estes testes travam as duas recusas que impedem os modos de
// falha SILENCIOSOS medidos na F89 — degradar para SQLite sem pedir, e deixar
// um segundo processo virar zumbi.

func TestClusterMode_PadraoESingle(t *testing.T) {
	t.Setenv(envClusterMode, "")
	modo, err := clusterModeConfigurado()
	if err != nil {
		t.Fatalf("erro inesperado com a variavel ausente: %v", err)
	}
	if modo != clusterModeSingle {
		t.Errorf("modo = %q com variavel ausente, quero %q", modo, clusterModeSingle)
	}
}

// TestClusterMode_ValorDesconhecidoEErro é a diferença entre este mecanismo e
// os que já falharam neste repo: valor inválido NÃO cai no padrão.
//
// `WA_API_CLUSTER_MODE=mutli` num manifesto de k8s cairia em `single` se o
// padrão fosse tolerante, e o operador teria N réplicas achando cada uma que é
// dona de tudo — exatamente o zumbi silencioso do Exp 1 da F89, multiplicado.
func TestClusterMode_ValorDesconhecidoEErro(t *testing.T) {
	for _, v := range []string{"mutli", "cluster", "1", "true", "sim"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv(envClusterMode, v)
			if _, err := clusterModeConfigurado(); err == nil {
				t.Errorf("valor %q foi aceito; deveria ser erro de arranque", v)
			}
		})
	}
}

func TestClusterMode_AceitaOsDoisValidos(t *testing.T) {
	for _, v := range []string{"single", "multi", "  MULTI  ", "Single"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv(envClusterMode, v)
			modo, err := clusterModeConfigurado()
			if err != nil {
				t.Fatalf("valor %q recusado: %v", v, err)
			}
			if modo != clusterModeSingle && modo != clusterModeMulti {
				t.Errorf("modo = %q", modo)
			}
		})
	}
}

// TestValidarStack_MultiExigePostgres trava a recusa que a produção precisaria
// hoje: ela roda em SQLite por queda automática, com as variáveis de Postgres
// PARCIALMENTE definidas. Em `multi` isso não pode ser um `warn`.
func TestValidarStack_MultiExigePostgres(t *testing.T) {
	if err := validarStackDoModo(clusterModeMulti, "sqlite"); err == nil {
		t.Fatal("multi com sqlite foi aceito; tem de ser erro fatal de arranque")
	} else if !strings.Contains(err.Error(), "DB_USER") {
		// A mensagem tem de dizer o que fazer. Um erro que so' informa que
		// falhou obriga o operador a ler o codigo.
		t.Errorf("mensagem nao diz quais variaveis definir: %q", err)
	}
	if err := validarStackDoModo(clusterModeMulti, "postgres"); err != nil {
		t.Errorf("multi com postgres recusado: %v", err)
	}
}

// TestValidarStack_SingleAceitaOsDois: o cenário catastrófico é modo suportado.
// SQLite com um pod tem de passar — é o padrão, não a exceção.
func TestValidarStack_SingleAceitaOsDois(t *testing.T) {
	for _, banco := range []string{"sqlite", "postgres"} {
		if err := validarStackDoModo(clusterModeSingle, banco); err != nil {
			t.Errorf("single com %s recusado: %v", banco, err)
		}
	}
}

// TestTravaInstancia_SegundaFalha reproduz, em teste, o cenário do Exp 1 da
// F89: dois processos sobre a mesma instalação.
//
// Medido lá: o WhatsApp derruba um, o perdedor fica com a sessão morta e nunca
// reconecta, e o banco continua dizendo `connected=1`. Aqui o segundo nem
// chega a subir.
func TestTravaInstancia_SegundaFalha(t *testing.T) {
	dir := t.TempDir()

	liberar, err := travarInstanciaUnica(dir)
	if err != nil {
		t.Fatalf("primeira trava falhou: %v", err)
	}
	defer liberar()

	// A segunda tentativa NO MESMO processo usa outro descritor de arquivo, que
	// e' o que o flock distingue — e' o mesmo que um segundo processo veria.
	if _, err := travarInstanciaUnica(dir); err == nil {
		t.Fatal("segunda trava sobre o mesmo diretorio foi concedida; o zumbi da F89 continua possivel")
	} else if !strings.Contains(err.Error(), envClusterMode) {
		t.Errorf("mensagem nao aponta a saida (%s=multi): %q", envClusterMode, err)
	}
}

// TestTravaInstancia_LiberaEPermiteOutra: a trava não pode virar cadeado
// permanente. Um reinício gracioso tem de conseguir subir de novo — e a
// medição da F89 mostrou que o religamento leva 1–2s, então qualquer atraso
// aqui seria perceptível.
func TestTravaInstancia_LiberaEPermiteOutra(t *testing.T) {
	dir := t.TempDir()

	liberar, err := travarInstanciaUnica(dir)
	if err != nil {
		t.Fatalf("primeira trava falhou: %v", err)
	}
	liberar()

	liberar2, err := travarInstanciaUnica(dir)
	if err != nil {
		t.Fatalf("apos liberar, a trava foi negada: %v", err)
	}
	liberar2()
}

// TestTravaInstancia_DiretoriosDistintosNaoConflitam: duas INSTALAÇÕES na
// mesma máquina são legítimas — foi assim que a validação da F89 rodou, com a
// produção num diretório e o teste noutro.
func TestTravaInstancia_DiretoriosDistintosNaoConflitam(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()

	la, err := travarInstanciaUnica(a)
	if err != nil {
		t.Fatalf("trava em A falhou: %v", err)
	}
	defer la()

	lb, err := travarInstanciaUnica(b)
	if err != nil {
		t.Fatalf("trava em B negada por causa de A; instalacoes distintas nao podem conflitar: %v", err)
	}
	defer lb()
}

func TestTravaInstancia_CriaODiretorioSeFaltar(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ainda", "nao", "existe")
	liberar, err := travarInstanciaUnica(dir)
	if err != nil {
		t.Fatalf("nao criou o diretorio de dados: %v", err)
	}
	defer liberar()
	if _, err := os.Stat(filepath.Join(dir, arquivoTravaInstancia)); err != nil {
		t.Errorf("arquivo de trava nao existe: %v", err)
	}
}

// TestPrepararCluster_MultiNaoTrava: em `multi` a exclusividade é por sessão
// (lease, D2), não por instalação. Travar a instalação ali impediria a segunda
// réplica de subir, que é justamente o ponto do modo.
func TestPrepararCluster_MultiNaoTrava(t *testing.T) {
	t.Setenv(envClusterMode, clusterModeMulti)
	dir := t.TempDir()

	l1, err := prepararCluster(dir, "postgres")
	if err != nil {
		t.Fatalf("multi+postgres recusado: %v", err)
	}
	defer l1()

	l2, err := prepararCluster(dir, "postgres")
	if err != nil {
		t.Fatalf("segunda replica em multi foi bloqueada pela trava de instalacao: %v", err)
	}
	defer l2()
}

func TestPrepararCluster_SingleTrava(t *testing.T) {
	t.Setenv(envClusterMode, clusterModeSingle)
	dir := t.TempDir()

	l1, err := prepararCluster(dir, "sqlite")
	if err != nil {
		t.Fatalf("single+sqlite recusado; e' o cenario catastrofico suportado: %v", err)
	}
	defer l1()

	if _, err := prepararCluster(dir, "sqlite"); err == nil {
		t.Error("segundo processo em single foi aceito")
	}
}
