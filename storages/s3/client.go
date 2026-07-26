package s3

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/x64c/gwf/gw/storages"
)

// ClientConf holds shared S3 credentials and region.
type ClientConf struct {
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

// Client implements storages.Client for AWS S3.
// One Client = one account + one region = one s3.Client.
type Client struct {
	client   *s3.Client
	storages map[string]*Storage
}

func NewClient(conf ClientConf) *Client {
	client := s3.New(s3.Options{
		Region: conf.Region,
		Credentials: aws.NewCredentialsCache(
			credentials.NewStaticCredentialsProvider(conf.AccessKeyID, conf.SecretAccessKey, ""),
		),
	})
	return &Client{
		client:   client,
		storages: make(map[string]*Storage),
	}
}

func (c *Client) CreateStorage(name string, rawConf jsontext.Value) error {
	var storageConf struct {
		Bucket string `json:"bucket"`
	}
	if err := json.Unmarshal(rawConf, &storageConf); err != nil {
		return fmt.Errorf("s3 storage: %w", err)
	}
	if storageConf.Bucket == "" {
		return fmt.Errorf("s3 storage: bucket is required")
	}
	if _, exists := c.storages[name]; exists {
		return fmt.Errorf("s3 storage: %q already exists", name)
	}
	c.storages[name] = &Storage{client: c.client, bucket: storageConf.Bucket}
	return nil
}

func (c *Client) Storage(name string) (storages.Storage, bool) {
	s, ok := c.storages[name]
	return s, ok
}

// PrepareClients loads S3 client configs from .storage-clients-s3.json
// and registers them into the provided client map.
func PrepareClients(appRoot string, clients map[string]storages.Client) error {
	confBytes, err := os.ReadFile(filepath.Join(appRoot, "config", ".storage-clients-s3.json"))
	if err != nil {
		return err
	}
	var confs map[string]ClientConf
	if err = json.Unmarshal(confBytes, &confs); err != nil {
		return err
	}
	for name, conf := range confs {
		clients[name] = NewClient(conf)
	}
	return nil
}
