package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Chris-Alexander-Pop/co/internal/queue"
)

type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

func New(base, token string) *Client {
	return &Client{
		Base:  base,
		Token: token,
		HTTP:  &http.Client{Timeout: 0},
	}
}

func (c *Client) Healthy() error {
	req, err := http.NewRequest(http.MethodGet, c.Base+"/health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	hc := &http.Client{Timeout: 3 * time.Second}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("builder status %s", res.Status)
	}
	return nil
}

type meta struct {
	Kind  string   `json:"kind"`
	Name  string   `json:"name"`
	Jobs  int      `json:"jobs"`
	Build []string `json:"build,omitempty"`
	Pull  []string `json:"pull,omitempty"`
}

func (c *Client) SubmitAur(name string) (*queue.Job, error) {
	return c.post(meta{Kind: queue.KindAur, Name: name}, nil)
}

func (c *Client) SubmitTree(kind, name string, jobs int, tarGz []byte) (*queue.Job, error) {
	return c.post(meta{Kind: kind, Name: name, Jobs: jobs}, tarGz)
}

func (c *Client) SubmitStrategy(name string, jobs int, build, pull []string, tarGz []byte) (*queue.Job, error) {
	return c.post(meta{Kind: queue.KindStrategy, Name: name, Jobs: jobs, Build: build, Pull: pull}, tarGz)
}

func (c *Client) post(m meta, tarGz []byte) (*queue.Job, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	mb, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if err := w.WriteField("meta", string(mb)); err != nil {
		return nil, err
	}
	if tarGz != nil {
		part, err := w.CreateFormFile("tree", "tree.tar.gz")
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(tarGz); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.Base+"/v1/jobs", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("submit: %s: %s", res.Status, bytes.TrimSpace(b))
	}
	var job queue.Job
	if err := json.NewDecoder(res.Body).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) Get(id string) (*queue.Job, error) {
	req, err := c.authReq(http.MethodGet, "/v1/jobs/"+id, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("job %s: %s", id, res.Status)
	}
	var job queue.Job
	if err := json.NewDecoder(res.Body).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) Log(id string) (string, error) {
	req, err := c.authReq(http.MethodGet, "/v1/jobs/"+id+"/log", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("log %s: %s", id, res.Status)
	}
	return string(b), nil
}

func (c *Client) Artifacts(id string) ([]string, error) {
	req, err := c.authReq(http.MethodGet, "/v1/jobs/"+id+"/artifacts", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artifacts %s: %s", id, res.Status)
	}
	var names []string
	if err := json.NewDecoder(res.Body).Decode(&names); err != nil {
		return nil, err
	}
	return names, nil
}

func (c *Client) Download(id, name, dest string) error {
	req, err := c.authReq(http.MethodGet, "/v1/jobs/"+id+"/artifacts/"+name, nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", name, res.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func (c *Client) Host() (queue.HostInfo, error) {
	req, err := c.authReq(http.MethodGet, "/v1/host", nil)
	if err != nil {
		return queue.HostInfo{}, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return queue.HostInfo{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return queue.HostInfo{}, fmt.Errorf("host: %s", res.Status)
	}
	var info queue.HostInfo
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return queue.HostInfo{}, err
	}
	return info, nil
}

func (c *Client) List() ([]queue.Job, error) {
	req, err := c.authReq(http.MethodGet, "/v1/jobs", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list: %s", res.Status)
	}
	var jobs []queue.Job
	if err := json.NewDecoder(res.Body).Decode(&jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (c *Client) authReq(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.Base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return req, nil
}
