package wanoise

import "time"

// Leitores dos dois contadores de reconexao. Os campos por tras deles sao
// atomicos e privados porque handleConnectSuccess e autoReconnect os tocam de
// goroutines diferentes (F46 em HOUSEKEEP.md); estes metodos sao o unico
// caminho de leitura.

// LastSuccessfulConnect devolve o instante da ultima autenticacao bem
// sucedida, ou o zero de time.Time se ainda nao houve nenhuma.
func (cli *Client) LastSuccessfulConnect() time.Time {
	nanos := cli.lastSuccessfulConnectUnixNano.Load()
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}

// AutoReconnectErrors devolve quantas tentativas de reconexao automatica
// falharam desde a ultima conexao bem sucedida. E' o numero que o
// AutoReconnectHook usa para decidir se ainda vale tentar.
func (cli *Client) AutoReconnectErrors() int {
	return int(cli.autoReconnectErrors.Load())
}
