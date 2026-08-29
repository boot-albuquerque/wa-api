package client

import "time"

// RequestTimeout é o teto de espera de uma requisição ao servidor do WhatsApp
// feita por um adapter (blocklist, privacidade, contatos). O SDK não impõe
// teto próprio: sem isto, um servidor que aceita a conexão e não responde
// prende o handler HTTP até o cliente desistir.
//
// Vive aqui, junto do seam, porque é característica da chamada ao cliente —
// não de um domínio em particular — e é consumido por mais de um subpacote
// de adapter (user/, misc/).
const RequestTimeout = 30 * time.Second
