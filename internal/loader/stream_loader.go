package loader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"starrock-benchmark/internal/config"
	"starrock-benchmark/internal/generator"
)

type StreamLoader struct {
	cfg               config.StarRocksConfig
	client            *http.Client
	isUpdate          bool
	partialUpdateMode string
}

func NewStreamLoader(cfg config.StarRocksConfig, isUpdate bool, partialUpdateMode string, maxConns int) *StreamLoader {
	user, pass := cfg.User, cfg.Password
	feHost := cfg.Host
	feHTTPPort := cfg.HTTPPort

	transport := &http.Transport{
		MaxConnsPerHost:     maxConns,
		MaxIdleConnsPerHost: maxConns,
		MaxIdleConns:        maxConns,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			req.SetBasicAuth(user, pass)

			if !strings.HasPrefix(req.URL.Host, feHost) {
				port := req.URL.Port()
				if port == "" {
					port = fmt.Sprintf("%d", feHTTPPort)
				}
				req.URL.Host = fmt.Sprintf("%s:%s", feHost, port)
			}
			return nil
		},
	}

	return &StreamLoader{
		cfg:               cfg,
		client:            client,
		isUpdate:          isUpdate,
		partialUpdateMode: partialUpdateMode,
	}
}

type streamLoadResponse struct {
	Status           string `json:"Status"`
	NumberLoadedRows int    `json:"NumberLoadedRows"`
	Message          string `json:"Message"`
	ErrorURL         string `json:"ErrorURL"`
}

func (s *StreamLoader) Load(ctx context.Context, schema generator.TableSchema, batch *generator.Batch) (int, error) {
	jsonData := buildJSON(schema, batch)
	loadURL := s.cfg.StreamLoadURL(schema.Database, schema.Table)
	label := fmt.Sprintf("bench_%s_%s_%d", schema.Database, schema.Table, time.Now().UnixNano())

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, loadURL, bytes.NewReader(jsonData))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}

	req.SetBasicAuth(s.cfg.User, s.cfg.Password)
	req.Header.Set("Expect", "100-continue")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("format", "json")
	req.Header.Set("strip_outer_array", "true")
	req.Header.Set("timeout", "600")
	req.Header.Set("strict_mode", "false")
	req.Header.Set("label", label)

	colNames := make([]string, 0, 2+len(schema.Columns))
	colNames = append(colNames, "id", "created_at")
	for _, col := range schema.Columns {
		colNames = append(colNames, col.Name)
	}
	req.Header.Set("columns", strings.Join(colNames, ","))

	if s.isUpdate {
		req.Header.Set("partial_update", "true")
		req.Header.Set("partial_update_mode", s.partialUpdateMode)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			return 0, fmt.Errorf("HTTP error (url=%s): %w", urlErr.URL, urlErr.Err)
		}
		return 0, fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("reading response (status=%d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var result streamLoadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parsing JSON response: %w (body: %s)",
			err, truncate(string(body), 500))
	}

	switch result.Status {
	case "Success", "Publish Timeout", "Label Already Exists":
		return result.NumberLoadedRows, nil
	default:
		errDetail := result.Message
		if result.ErrorURL != "" {
			errDetail += " (error_url: " + result.ErrorURL + ")"
		}
		return 0, fmt.Errorf("stream load [%s]: %s", result.Status, errDetail)
	}
}

func (s *StreamLoader) Close() error {
	s.client.CloseIdleConnections()
	return nil
}

func buildJSON(schema generator.TableSchema, batch *generator.Batch) []byte {
	colNames := make([]string, 0, 2+len(schema.Columns))
	colNames = append(colNames, "id", "created_at")
	for _, col := range schema.Columns {
		colNames = append(colNames, col.Name)
	}

	rows := make([]map[string]interface{}, 0, len(batch.Rows))
	for _, row := range batch.Rows {
		obj := make(map[string]interface{}, len(colNames))
		for i, colName := range colNames {
			if i < len(row) {
				obj[colName] = formatJSONValue(row[i])
			}
		}
		rows = append(rows, obj)
	}

	data, _ := json.Marshal(rows)
	return data
}

func formatJSONValue(v interface{}) interface{} {
	switch val := v.(type) {
	case time.Time:
		return val.Format("2006-01-02 15:04:05")
	default:
		return val
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
