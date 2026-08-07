package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"go.mau.fi/util/random"

	"wa-api/internal/wa-noise/protocol/socket"
	"wa-api/internal/wa-noise/security/cbc"
)

// UploadResponse tem os dados do upload do anexo, que devem ser copiados para
// os campos correspondentes da mensagem a enviar.
type UploadResponse struct {
	URL        string `json:"url"`
	DirectPath string `json:"direct_path"`
	Handle     string `json:"handle"`
	ObjectID   string `json:"object_id"`

	MediaKey      []byte `json:"-"`
	FileEncSHA256 []byte `json:"-"`
	FileSHA256    []byte `json:"-"`
	FileLength    uint64 `json:"-"`
}

// Upload cifra e sobe o anexo dado aos servidores do WhatsApp.
func Upload(ctx context.Context, t Transport, plaintext []byte, appInfo Type) (resp UploadResponse, err error) {
	resp.FileLength = uint64(len(plaintext))
	resp.MediaKey = random.Bytes(mediaKeyLength)

	plaintextSHA256 := sha256.Sum256(plaintext)
	resp.FileSHA256 = plaintextSHA256[:]

	iv, cipherKey, macKey, _ := GetKeys(resp.MediaKey, appInfo)

	var ciphertext []byte
	ciphertext, err = cbcutil.Encrypt(cipherKey, iv, plaintext)
	if err != nil {
		err = fmt.Errorf("failed to encrypt file: %w", err)
		return
	}

	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(ciphertext)
	dataToUpload := append(ciphertext, h.Sum(nil)[:mediaHMACLength]...)

	dataHash := sha256.Sum256(dataToUpload)
	resp.FileEncSHA256 = dataHash[:]

	err = RawUpload(ctx, t, bytes.NewReader(dataToUpload), uint64(len(dataToUpload)), resp.FileEncSHA256, appInfo, false, &resp)
	return
}

// UploadReader e' identico a [Upload], mas le o plaintext de um io.Reader.
//
// O processo de cifragem precisa de um arquivo temporario. Se tempFile for nil,
// um e' criado e removido ao fim. Para usar um unico arquivo, passe o mesmo
// arquivo como plaintext e tempFile — ele sera' sobrescrito com o ciphertext.
func UploadReader(
	ctx context.Context, t Transport, plaintext io.Reader, tempFile io.ReadWriteSeeker, appInfo Type,
) (resp UploadResponse, err error) {
	resp.MediaKey = random.Bytes(mediaKeyLength)
	iv, cipherKey, macKey, _ := GetKeys(resp.MediaKey, appInfo)
	if tempFile == nil {
		tempFile, err = os.CreateTemp("", "whatsmeow-upload-*")
		if err != nil {
			err = fmt.Errorf("failed to create temporary file: %w", err)
			return
		}
		defer func() {
			tempFileFile := tempFile.(*os.File)
			_ = tempFileFile.Close()
			_ = os.Remove(tempFileFile.Name())
		}()
	}
	var uploadSize uint64
	resp.FileSHA256, resp.FileEncSHA256, resp.FileLength, uploadSize, err = cbcutil.EncryptStream(cipherKey, iv, macKey, plaintext, tempFile)
	if err != nil {
		err = fmt.Errorf("failed to encrypt file: %w", err)
		return
	}
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		err = fmt.Errorf("failed to seek to start of temporary file: %w", err)
		return
	}
	err = RawUpload(ctx, t, tempFile, uploadSize, resp.FileEncSHA256, appInfo, false, &resp)
	return
}

// uploadTarget descreve para onde o POST de upload vai: mms-type efetivo,
// prefixo de caminho e host escolhido na media connection.
type uploadTarget struct {
	mmsType string
	prefix  string
	host    string
}

// resolveUploadTarget aplica as regras de Messenger e de newsletter sobre o
// mms-type, o prefixo de caminho e a escolha de host.
func resolveUploadTarget(t Transport, mediaConn *Conn, appInfo Type, newsletter bool) uploadTarget {
	target := uploadTarget{
		mmsType: mediaTypeToMMSType[appInfo],
		prefix:  uploadPrefixDefault,
	}
	if t.IsMessenger() {
		target.prefix = uploadPrefixMessenger
		// Messenger upload only allows voice messages, not audio files
		if target.mmsType == mmsTypeAudio {
			target.mmsType = mmsTypePTT
		}
	}
	if newsletter {
		target.mmsType = fmt.Sprintf(newsletterMMSTypeFormat, target.mmsType)
		target.prefix = uploadPrefixNewsletter
	}
	// Hacky hack to prefer last option (rupload.facebook.com) for messenger uploads.
	// For some reason, the primary host doesn't work, even though it has the <upload/> tag.
	if t.IsMessenger() {
		target.host = mediaConn.Hosts[len(mediaConn.Hosts)-1].Hostname
	} else {
		target.host = mediaConn.Hosts[0].Hostname
	}
	return target
}

// RawUpload faz o POST do conteudo ja' preparado e decodifica a resposta em
// resp.
func RawUpload(
	ctx context.Context, t Transport, dataToUpload io.Reader, uploadSize uint64,
	fileHash []byte, appInfo Type, newsletter bool, resp *UploadResponse,
) error {
	mediaConn, err := RefreshConn(ctx, t, false)
	if err != nil {
		return fmt.Errorf("failed to refresh media connections: %w", err)
	}

	token := base64.URLEncoding.EncodeToString(fileHash)
	q := url.Values{
		"auth":  []string{mediaConn.Auth},
		"token": []string{token},
	}
	target := resolveUploadTarget(t, mediaConn, appInfo, newsletter)
	uploadURL := url.URL{
		Scheme:   "https",
		Host:     target.host,
		Path:     fmt.Sprintf(uploadPathFormat, target.prefix, target.mmsType, token),
		RawQuery: q.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), dataToUpload)
	if err != nil {
		return fmt.Errorf("failed to prepare request: %w", err)
	}

	req.ContentLength = int64(uploadSize)
	req.Header.Set("Origin", socket.Origin)
	req.Header.Set("Referer", socket.Origin+"/")

	httpResp, err := t.HTTPClient().Do(req)
	if err != nil {
		err = fmt.Errorf("failed to execute request: %w", err)
	} else if httpResp.StatusCode != http.StatusOK {
		err = fmt.Errorf("upload failed with status code %d", httpResp.StatusCode)
	} else if err = json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		err = fmt.Errorf("failed to parse upload response: %w", err)
	}
	if httpResp != nil {
		_ = httpResp.Body.Close()
	}
	return err
}

// Delete apaga a midia no directPath dado nos servidores do WhatsApp.
//
// So' e' usado em coisas como history sync, que devem ser apagadas apos o
// processamento.
func Delete(ctx context.Context, t Transport, appInfo Type, directPath string, encFileHash []byte, encHandle string) error {
	mediaConn, err := RefreshConn(ctx, t, false)
	if err != nil {
		return fmt.Errorf("failed to refresh media connections: %w", err)
	}

	queryStart := strings.IndexByte(directPath, '?')
	if queryStart > 0 {
		directPath = directPath[:queryStart]
	}

	token := base64.URLEncoding.EncodeToString(encFileHash)
	query := url.Values{
		"token": []string{token},
		"d_md":  []string{base64.RawURLEncoding.EncodeToString([]byte(directPath))},
		"auth":  []string{mediaConn.Auth},
	}
	if encHandle != "" {
		query.Set("e_handle", encHandle)
	}
	deleteURL := url.URL{
		Scheme:   "https",
		Host:     mediaConn.Hosts[0].Hostname,
		Path:     fmt.Sprintf(deletePathFormat, mediaTypeToMMSType[appInfo], token),
		RawQuery: query.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to prepare request: %w", err)
	}

	req.Header.Set("Origin", socket.Origin)
	req.Header.Set("Referer", socket.Origin+"/")
	// TODO non-on-demand backfills may require this? it's in the initial bootstrap payload and may need to be persisted
	//req.Header.Set("Companion_User_Secret", companionMetaNonce)

	httpResp, err := t.HTTPClient().Do(req)
	if err != nil {
		err = fmt.Errorf("failed to execute request: %w", err)
	} else if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		err = fmt.Errorf("media delete failed with status code %d", httpResp.StatusCode)
	}
	if httpResp != nil {
		_ = httpResp.Body.Close()
	}
	return err
}
