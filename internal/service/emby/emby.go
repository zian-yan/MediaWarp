package emby

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/utils"
)

type Client struct {
	baseURL *url.URL
	apiKey  string
	client  *http.Client
}

func (client *Client) GetType() constants.MediaServerType {
	return constants.EMBY
}

func (client *Client) GetBaseURLString() string {
	return client.baseURL.String()
}

func (client *Client) GetAPIKey() string {
	return client.apiKey
}

func (client *Client) ItemsServiceQueryItem(ids string, limit int, fields string) (*EmbyResponse, error) {
	var (
		params       = url.Values{}
		itemResponse = &EmbyResponse{}
	)
	params.Add("Ids", ids)
	params.Add("Limit", strconv.Itoa(limit))
	params.Add("Fields", fields)
	params.Add("Recursive", "true")
	params.Add("api_key", client.GetAPIKey())
	api := client.baseURL.JoinPath("Items")
	api.RawQuery = params.Encode()
	resp, err := client.client.Get(api.String())
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, itemResponse)
	if err != nil {
		return nil, err
	}
	return itemResponse, nil
}

func New(baseURL string, apiKey string) (*Client, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("解析 baseURL 失败: %w", err)
	}
	client := &Client{
		baseURL: parsedURL,
		apiKey:  apiKey,
		client:  utils.GetHTTPClient(),
	}
	return client, nil
}
