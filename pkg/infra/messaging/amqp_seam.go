package messaging

import (
	"github.com/rabbitmq/amqp091-go"
)

// amqpChannel e' a superficie minima de *amqp091.Channel usada por este
// pacote. Existe para que o caminho de publicacao seja exercitavel sem um
// broker de verdade — nenhum comportamento muda, os metodos tem a assinatura
// exata do tipo concreto.
type amqpChannel interface {
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error)
	Publish(exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error
}

// amqpConnection e' a superficie minima de *amqp091.Connection usada aqui.
// `openChannel` e' deliberadamente nao-exportado: e' o unico metodo que nao
// casa com o tipo concreto (o SDK devolve *amqp091.Channel), entao a conexao
// real entra pelo adaptador realConn abaixo.
type amqpConnection interface {
	Close() error
	NotifyClose(receiver chan *amqp091.Error) chan *amqp091.Error
	openChannel() (amqpChannel, error)
}

// realConn adapta *amqp091.Connection a amqpConnection.
type realConn struct {
	*amqp091.Connection
}

func (c realConn) openChannel() (amqpChannel, error) { return c.Connection.Channel() }

// dialAMQP e' o unico ponto do pacote que fala com o SDK. Os testes o
// substituem; producao usa amqp091.Dial.
var dialAMQP = func(url string) (amqpConnection, error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, err
	}
	return realConn{conn}, nil
}
