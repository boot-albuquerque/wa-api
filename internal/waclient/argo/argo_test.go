package argo

import "testing"

func TestInit(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init() erro inesperado: %v", err)
	}
	if len(Store) == 0 {
		t.Error("Store vazio após Init()")
	}
	if len(QueryIDToMessageName) == 0 {
		t.Error("QueryIDToMessageName vazio após Init()")
	}
}

func TestInit_Idempotente(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("primeira chamada: erro inesperado: %v", err)
	}
	storeRef := Store
	if err := Init(); err != nil {
		t.Fatalf("segunda chamada: erro inesperado: %v", err)
	}
	for id := range storeRef {
		if _, ok := Store[id]; !ok {
			t.Errorf("Store mudou entre chamadas: chave %q sumiu", id)
		}
		break
	}
}

func TestGetStore(t *testing.T) {
	store, err := GetStore()
	if err != nil {
		t.Fatalf("GetStore() erro inesperado: %v", err)
	}
	if len(store) == 0 {
		t.Error("GetStore() retornou mapa vazio")
	}
}

func TestGetQueryIDToMessageName(t *testing.T) {
	m, err := GetQueryIDToMessageName()
	if err != nil {
		t.Fatalf("GetQueryIDToMessageName() erro inesperado: %v", err)
	}
	if len(m) == 0 {
		t.Error("GetQueryIDToMessageName() retornou mapa vazio")
	}
}

func TestQueryIDToMessageName_InversaoDoJSON(t *testing.T) {
	if _, err := GetQueryIDToMessageName(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	for id, name := range QueryIDToMessageName {
		if id == "" || name == "" {
			t.Errorf("entrada com chave ou valor vazio: id=%q name=%q", id, name)
		}
	}
}
