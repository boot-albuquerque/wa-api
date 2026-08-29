package binary

// Constantes do formato binario/XML do WhatsApp que antes apareciam como
// literais soltos no encoder e no decoder.
//
// O que NAO esta aqui, de proposito: os valores de tag do protocolo
// (ListEmpty, Binary8, Nibble8, Dictionary0...), que ja' sao constantes
// nomeadas em binary/token, e os limites PackedMax/SingleByteMax, que tambem
// moram la'. Este arquivo cobre so' o que sobrou sem nome.
const (
	// bitsPerByte e byteMask sao a aritmetica de deslocamento usada para
	// serializar inteiros byte a byte (readIntN/pushIntN).
	bitsPerByte = 8
	byteMask    = 0xFF

	// Largura em bytes de cada inteiro do formato.
	int8Size  = 1
	int16Size = 2
	int20Size = 3
	int32Size = 4
)

// Um inteiro de 20 bits ocupa 3 bytes: os 4 bits baixos do primeiro byte mais
// os dois bytes seguintes.
const (
	int20HighNibbleMask = 0x0F
	int20HighShift      = 2 * bitsPerByte
	int20MiddleShift    = bitsPerByte
	int20Max            = 1 << 20
)

// Empacotamento nibble8/hex8: cada byte carrega dois caracteres, um em cada
// meia-byte (nibble).
const (
	nibbleShift        = 4
	nibbleHighMask     = 0xF0
	nibbleLowMask      = 0x0F
	charsPerPackedByte = 2

	// packedOddLengthFlag e' o bit alto do byte de tamanho do bloco
	// empacotado: quando ligado, o ultimo nibble e' padding e o leitor
	// descarta o caractere final. packedLengthMask isola o tamanho.
	packedOddLengthFlag = 0x80
	packedLengthMask    = 0x7F
)

// Sentinelas do alfabeto nibble8. Os digitos '0'-'9' ocupam 0-9; os tres
// valores abaixo estendem o alfabeto.
const (
	nibbleDash    = 10
	nibbleDot     = 11
	nibblePadding = 15
)

// Tamanho dos alfabetos posicionais usados por nibble8 e hex8.
const (
	decimalDigitCount = 10
	hexDigitCount     = 16
)

const (
	// tagSize e' a posicao que a tag do elemento ocupa na lista que
	// representa um Node.
	tagSize = 1

	// attrEntrySize e' quantas posicoes da mesma lista cada atributo ocupa
	// (chave + valor). E' o que torna o tamanho da lista par quando o no'
	// tem conteudo e impar quando nao tem.
	attrEntrySize = 2

	// emptyNodeTag e' a tag que o encoder trata como "no' vazio": em vez de
	// serializar o elemento, escreve uma lista de um item vazio. E' o token
	// "0" da tabela de token single-byte.
	emptyNodeTag = "0"
)

// zlibCompressedFlag e' o bit do primeiro byte de um frame que indica que o
// restante esta comprimido com zlib (ver Unpack).
const zlibCompressedFlag = 2
