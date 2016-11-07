package thumbnail

import (
	"fmt"
	"github.com/die-net/fotomat/format"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

const (
	imageDirectory = "../testdata/"
)

func TestSuccess(t *testing.T) {
	ps := newProxyServer(time.Minute)
	defer ps.close()

	ps.options = Options{Save: format.SaveOptions{Lossless: true}}
	assert.Nil(t, ps.isSize("2px.png", format.Png, 2, 3))

	// Crop JPEG to 200x100 and convert to WebP.
	ps.options = Options{Width: 200, Height: 100, Crop: true, Save: format.SaveOptions{AllowWebp: true}}
	assert.Nil(t, ps.isSize("watermelon.jpg", format.Webp, 200, 100))
}

func TestTimeout(t *testing.T) {
	ps := newProxyServer(time.Nanosecond)
	defer ps.close()

	body, status := ps.get("timeout")
	assert.Equal(t, http.StatusGatewayTimeout, status, string(body))
}

func TestErrors(t *testing.T) {
	ps := newProxyServer(time.Minute)
	defer ps.close()

	// Return StatusNotFound on a textfile that doesn't exist.
	assert.Equal(t, ps.getStatus("notfound.txt"), http.StatusNotFound)

	// Return StatusUnsupportedMediaType on a text file.
	assert.Equal(t, ps.getStatus("notimage.txt"), http.StatusUnsupportedMediaType)

	// Return StatusUnsupportedMediaType on a truncated image.
	assert.Equal(t, ps.getStatus("bad.jpg"), http.StatusUnsupportedMediaType)

	// Return StatusUnsupportedMediaType on a 1x1 pixel image.
	assert.Equal(t, ps.getStatus("1px.png"), http.StatusUnsupportedMediaType)

	// Return StatusRequestEntityTooLarge on a 34000px image.
	assert.Equal(t, ps.getStatus("34000px.png"), http.StatusRequestEntityTooLarge)

	// Make sure director return status is working
	ps.status = 403
	assert.Equal(t, ps.getStatus("2px.png"), 403)

	// Make sure NewProxy returns nil on bad input
	assert.Nil(t, NewProxy(nil, nil, 0, nil))
}

type proxyServer struct {
	proxy   *Proxy
	server  *httptest.Server
	origin  *httptest.Server
	options Options
	status  int
	scheme  string
	host    string
}

func newProxyServer(timeout time.Duration) *proxyServer {
	// Static http server that serves our test images
	origin := httptest.NewServer(http.FileServer(http.Dir(imageDirectory)))
	url, err := url.Parse(origin.URL)
	if err != nil {
		panic("Bad origin URL")
	}

	ps := &proxyServer{
		origin: origin,
		scheme: url.Scheme,
		host:   url.Host,
	}

	// Proxy http server that fetches and thumbnails images from origin
	ps.proxy = NewProxy(ps.director, NewPool(0, 1), 2, &http.Client{Timeout: timeout})
	ps.server = httptest.NewServer(ps.proxy)

	return ps
}

func (ps *proxyServer) director(req *http.Request) (Options, int) {
	req.URL.Scheme = ps.scheme
	req.URL.Host = ps.host
	return ps.options, ps.status
}

func (ps *proxyServer) get(filename string) ([]byte, int) {
	url := ps.server.URL + "/" + filename
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		panic(err)
	}

	return body, resp.StatusCode
}

func (ps *proxyServer) getStatus(filename string) int {
	_, code := ps.get(filename)
	return code
}

func (ps *proxyServer) isSize(filename string, f format.Format, width, height int) error {
	image, code := ps.get(filename)
	if code != 200 {
		return fmt.Errorf("HTTP error %d: %s", code, string(image))
	}

	m, err := format.MetadataBytes(image)
	if err != nil {
		return err
	}
	if m.Width != width || m.Height != height {
		return fmt.Errorf("Width %d!=%d or Height %d!=%d", m.Width, width, m.Height, height)
	}
	if m.Format != f {
		return fmt.Errorf("Format %s!=%s", m.Format, f)
	}
	return nil
}

func (ps *proxyServer) close() {
	ps.server.Close()
	ps.proxy.Close()
	ps.origin.Close()
}
