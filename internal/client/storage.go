package client

import (
	"context"
	"io"

	"cloud.google.com/go/storage"
)

// StorageClient wraps the Google Cloud Storage client.
type StorageClient struct {
	client     *storage.Client
	bucketName string
}

// NewStorageClient creates a new storage client.
func NewStorageClient(ctx context.Context, bucketName string) (*StorageClient, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return &StorageClient{
		client:     client,
		bucketName: bucketName,
	}, nil
}

// Close closes the client.
func (c *StorageClient) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// Upload uploads data to cloud storage.
func (c *StorageClient) Upload(ctx context.Context, objectName string, data []byte) (string, error) {
	bucket := c.client.Bucket(c.bucketName)
	obj := bucket.Object(objectName)
	w := obj.NewWriter(ctx)

	if _, err := w.Write(data); err != nil {
		w.Close()
		return "", err
	}

	if err := w.Close(); err != nil {
		return "", err
	}

	// Return the public URL
	return "gs://" + c.bucketName + "/" + objectName, nil
}

// UploadReader uploads data from a reader to cloud storage.
func (c *StorageClient) UploadReader(ctx context.Context, objectName string, reader io.Reader) (string, error) {
	bucket := c.client.Bucket(c.bucketName)
	obj := bucket.Object(objectName)
	w := obj.NewWriter(ctx)

	if _, err := io.Copy(w, reader); err != nil {
		w.Close()
		return "", err
	}

	if err := w.Close(); err != nil {
		return "", err
	}

	return "gs://" + c.bucketName + "/" + objectName, nil
}

// Download downloads data from cloud storage.
func (c *StorageClient) Download(ctx context.Context, objectName string) ([]byte, error) {
	bucket := c.client.Bucket(c.bucketName)
	obj := bucket.Object(objectName)
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// DownloadToWriter downloads data from cloud storage to a writer.
func (c *StorageClient) DownloadToWriter(ctx context.Context, objectName string, writer io.Writer) error {
	bucket := c.client.Bucket(c.bucketName)
	obj := bucket.Object(objectName)
	r, err := obj.NewReader(ctx)
	if err != nil {
		return err
	}
	defer r.Close()

	_, err = io.Copy(writer, r)
	return err
}

// Delete deletes an object from cloud storage.
func (c *StorageClient) Delete(ctx context.Context, objectName string) error {
	bucket := c.client.Bucket(c.bucketName)
	obj := bucket.Object(objectName)
	return obj.Delete(ctx)
}

// Exists checks if an object exists in cloud storage.
func (c *StorageClient) Exists(ctx context.Context, objectName string) (bool, error) {
	bucket := c.client.Bucket(c.bucketName)
	obj := bucket.Object(objectName)
	_, err := obj.Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// List lists objects in the bucket with the given prefix.
func (c *StorageClient) List(ctx context.Context, prefix string) ([]string, error) {
	bucket := c.client.Bucket(c.bucketName)
	it := bucket.Objects(ctx, &storage.Query{Prefix: prefix})

	var objects []string
	for {
		attrs, err := it.Next()
		if err == storage.ErrObjectNotExist {
			break
		}
		if err != nil {
			return nil, err
		}
		objects = append(objects, attrs.Name)
	}

	return objects, nil
}
