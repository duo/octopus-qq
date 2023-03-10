package common

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

var (
	httpClient = &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2:   true,
			MaxConnsPerHost:     0,
			MaxIdleConns:        0,
			MaxIdleConnsPerHost: 256,
		},
	}

	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36 Edg/87.0.664.66"

	// created by JogleLew and jqqqqqqqqqq, optimized based on Tim's emoji support, updated by xzsk2 to mobileqq v8.8.11
	emojis = map[string]string{
		"NO":   "đĢ",
		"OK":   "đ",
		"ä¸åŧåŋ":  "đ",
		"äšäš":   "đ",
		"äžŋäžŋ":   "đŠ",
		"åˇįŦ":   "đ",
		"å˛æĸ":   "đ",
		"åč§":   "đ",
		"åˇæą":   "đ",
		"åč°ĸ":   "đĨ",
		"å":    "đĒ",
		"åå":   "đŗ",
		"åæ":   "đĄ",
		"åæ":   "đŽ",
		"å¯įą":   "đ",
		"åŗåŧåŧ":  "đ",
		"å":    "đ¨",
		"å":    "đ",
		"å˛į":   "đ",
		"åéĒ":   "đ¤",
		"ååĄ":   "âī¸",
		"åæŦ ":   "đĨą",
		"å¤é":   "đē",
		"åĩåĩ":   "đ",
		"ååĨļ":   "đŧ",
		"ååŊŠ":   "đ",
		"å":    "đ¤",
		"å°":    "đĒ",
		"åįŦ":   "đ",
		"å¤§å­":   "đ­",
		"å¤§įŦ":   "đ",
		"å¤Ēéŗ":   "đī¸",
		"åĨæ":   "â",
		"åĨŊæŖ":   "đ",
		"å§åą":   "đ­",
		"åŽŗæ":   "đ¨",
		"åŽŗįž":   "âēī¸",
		"å°´å°Ŧ":   "đ°",
		"åˇĻäē˛äē˛":  "đ",
		"åˇĻåŧåŧ":  "đ",
		"åš˛æ¯":   "đģ",
		"åšŊįĩ":   "đģ",
		"åŧæĒ":   "đĢ",
		"åžæ":   "đ",
		"åžŽįŦ":   "đ",
		"åŋįĸ":   "đī¸",
		"åŋĢå­äē":  "đ­",
		"æ é˛":   "đ¤",
		"æå":   "đŽ",
		"ææ":   "đ¨",
		"æčŽļ":   "đŽ",
		"æ¨įŦ":   "đŦ",
		"ææĒ":   "đĢ",
		"æį":   "đ¤",
		"æįŖ¨":   "đŠ",
		"æąæą":   "đ¤",
		"ææ":   "đ",
		"ææ":   "đ",
		"æĨæą":   "đ¤ˇ",
		"æŗå¤´":   "â",
		"æĨæ":   "đ",
		"æĄæ":   "đ¤",
		"æå´":   "đŖ",
		"æ˛æ":   "đ¨",
		"æ":    "đĩ",
		"æäēŽ":   "đ",
		"æŖæŖįŗ":  "đ­",
		"æ˛ŗčš":   "đĻ",
		"æŗĒåĨ":   "đ­",
		"æĩæą":   "đ",
		"æĩæŗĒ":   "đ­",
		"į¯įŦŧ":   "đŽ",
		"į¸åŧš":   "đŖ",
		"įščĩ":   "đ",
		"įąäŊ ":   "đ¤",
		"įąåŋ":   "â¤ī¸",
		"įąæ":   "đ",
		"įĒå¤´":   "đˇ",
		"įŽåģ":   "đ",
		"įĢį°":   "đš",
		"įĸčĢ":   "đ",
		"įæĨåŋĢäš": "đ",
		"įéŽ":   "đ¤",
		"įŊįŧ":   "đ",
		"įĄ":    "đ´",
		"į¤ēįą":   "â¤ī¸",
		"į¤ŧįŠ":   "đ",
		"įĨįĨˇ":   "đ",
		"įŦå­":   "đ",
		"į¯Žį":   "đ",
		"įēĸå":   "đ§§",
		"čåŠ":   "âī¸",
		"č˛":    "đ",
		"čļ":    "đĩ",
		"č¯":    "đ",
		"ččą":   "đŧ",
		"čå":   "đĒ",
		"č":    "đĨ",
		"čįŗ":   "đ",
		"čĄ°":    "đŖ",
		"čĨŋį":   "đ",
		"č°įŽ":   "đ",
		"čĩ":    "đ",
		"čļŗį":   "âŊī¸",
		"čˇŗčˇŗ":   "đē",
		"č¸Š":    "đ",
		"éčą":   "đ",
		"éˇ":    "đ¤",
		"éįĨ¨":   "đĩ",
		"éĒįĩ":   "âĄ",
		"é­å´":   "đˇ",
		"éžčŋ":   "đ",
		"é­įŽ":   "đ§¨",
		"éŖæŗĒ":   "đ­",
		"éŖåģ":   "đĨ°",
		"éŖæē":   "đŠ",
		"éĨĨéĨŋ":   "đ¤¤",
		"éĨ­":    "đ",
		"éĒˇéĢ":   "đ",
		"éŧæ":   "đ",
	}
)

func ConvertFace(face string) string {
	if val, ok := emojis[face]; ok {
		return val
	}
	return "/" + face
}

func Itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}

func Atoi(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func DelFile(path string) bool {
	err := os.Remove(path)
	return err == nil
}

func FileExist(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func Download(url string) (*BlobData, error) {
	data, err := GetBytes(url)
	if err != nil {
		return nil, err
	}

	return &BlobData{
		Mime:   mimetype.Detect(data).String(),
		Binary: data,
	}, nil
}

func GetBytes(url string) ([]byte, error) {
	reader, err := HTTPGetReadCloser(url)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()

	return io.ReadAll(reader)
}

type gzipCloser struct {
	f io.Closer
	r *gzip.Reader
}

func NewGzipReadCloser(reader io.ReadCloser) (io.ReadCloser, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}

	return &gzipCloser{
		f: reader,
		r: gzipReader,
	}, nil
}

func (g *gzipCloser) Read(p []byte) (n int, err error) {
	return g.r.Read(p)
}

func (g *gzipCloser) Close() error {
	_ = g.f.Close()

	return g.r.Close()
}

func HTTPGetReadCloser(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header["User-Agent"] = []string{UserAgent}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		return NewGzipReadCloser(resp.Body)
	}

	return resp.Body, err
}

// Request is a file download request
type Request struct {
	Method string
	URL    string
	Header map[string]string
	Limit  int64
	Body   io.Reader
}

func (r Request) do() (*http.Response, error) {
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	req, err := http.NewRequest(r.Method, r.URL, r.Body)
	if err != nil {
		return nil, err
	}

	req.Header["User-Agent"] = []string{UserAgent}
	for k, v := range r.Header {
		req.Header.Set(k, v)
	}

	return httpClient.Do(req)
}

func (r Request) body() (io.ReadCloser, error) {
	resp, err := r.do()
	if err != nil {
		return nil, err
	}

	limit := r.Limit // check file size limit
	if limit > 0 && resp.ContentLength > limit {
		_ = resp.Body.Close()
		return nil, errors.New("oversize")
	}

	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		return NewGzipReadCloser(resp.Body)
	}
	return resp.Body, err
}

func (r Request) Bytes() ([]byte, error) {
	rd, err := r.body()
	if err != nil {
		return nil, err
	}
	defer rd.Close()
	return io.ReadAll(rd)
}
