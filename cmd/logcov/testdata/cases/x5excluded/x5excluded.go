// Package x5excluded e' fixture da regra X5: o mesmo par de funcoes e'
// EXCLUDED quando o pacote esta' em .logcov-exclude e ELIGIBLE quando nao.
package x5excluded

import "errors"

func Alfa(flag bool) error {
	if flag {
		return errors.New("alfa")
	}
	return nil
}

func Beta(flag bool) error {
	if flag {
		return errors.New("beta")
	}
	return nil
}
