package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/intelsk/backend/models"
)

type MLClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewMLClient(baseURL string) *MLClient {
	return &MLClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // CPU CLIP inference is slow
		},
	}
}

func (c *MLClient) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}

func (c *MLClient) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := c.HealthCheck(); err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("ML sidecar not ready after %s", timeout)
}

func (c *MLClient) EncodeImages(paths []string) ([][]float64, error) {
	body, err := json.Marshal(map[string]any{"paths": paths})
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Post(c.baseURL+"/encode/image",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("encode images request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("encode images returned %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding encode images response: %w", err)
	}
	return result.Embeddings, nil
}

func (c *MLClient) EncodeText(text string) ([]float64, error) {
	body, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Post(c.baseURL+"/encode/text",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("encode text request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("encode text returned %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding encode text response: %w", err)
	}
	return result.Embedding, nil
}

type ModelInfo struct {
	Preset       string            `json:"preset"`
	Model        string            `json:"model"`
	EmbeddingDim int               `json:"embedding_dim"`
	Presets      map[string]string `json:"presets,omitempty"`
	Status       string            `json:"status,omitempty"`
}

func (c *MLClient) GetModelInfo() (*ModelInfo, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/model")
	if err != nil {
		return nil, fmt.Errorf("get model info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get model info returned %d: %s", resp.StatusCode, respBody)
	}

	var info ModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding model info response: %w", err)
	}
	return &info, nil
}

func (c *MLClient) ReloadModel(preset string) (*ModelInfo, error) {
	body, err := json.Marshal(map[string]string{"preset": preset})
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Post(c.baseURL+"/reload",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("reload model request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("reload model returned %d: %s", resp.StatusCode, respBody)
	}

	var info ModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding reload model response: %w", err)
	}
	return &info, nil
}

func (c *MLClient) SearchByText(dbPath, text string, cameraIDs []string,
	startTime, endTime string, limit int, minScore float64) ([]models.SearchResult, error) {
	body, err := json.Marshal(map[string]any{
		"db_path":    dbPath,
		"text":       text,
		"camera_ids": cameraIDs,
		"start_time": startTime,
		"end_time":   endTime,
		"limit":      limit,
		"min_score":  minScore,
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Post(c.baseURL+"/search/image",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search returned %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Results []models.SearchResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}
	return result.Results, nil
}
