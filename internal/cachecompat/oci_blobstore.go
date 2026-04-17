package cachecompat

import (
	"context"
	"fmt"
	"io"
	"sync"

	"ohoci/internal/oci"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

type ObjectStorageClient interface {
	GetNamespace(ctx context.Context, request objectstorage.GetNamespaceRequest) (objectstorage.GetNamespaceResponse, error)
	PutObject(ctx context.Context, request objectstorage.PutObjectRequest) (objectstorage.PutObjectResponse, error)
	GetObject(ctx context.Context, request objectstorage.GetObjectRequest) (objectstorage.GetObjectResponse, error)
	DeleteObject(ctx context.Context, request objectstorage.DeleteObjectRequest) (objectstorage.DeleteObjectResponse, error)
}

type OCIBlobStore struct {
	resolver      oci.ProviderResolver
	clientFactory func(common.ConfigurationProvider) (ObjectStorageClient, error)
	namespaceMu   sync.Mutex
	namespace     string
}

func NewOCIBlobStore(resolver oci.ProviderResolver) *OCIBlobStore {
	return &OCIBlobStore{
		resolver: resolver,
		clientFactory: func(provider common.ConfigurationProvider) (ObjectStorageClient, error) {
			return objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
		},
	}
}

func (s *OCIBlobStore) Put(ctx context.Context, bucketName, objectName string, body io.ReadSeeker, sizeBytes int64, contentType string) error {
	client, namespace, err := s.resolve(ctx)
	if err != nil {
		return err
	}
	_, err = client.PutObject(ctx, objectstorage.PutObjectRequest{
		NamespaceName: common.String(namespace),
		BucketName:    common.String(bucketName),
		ObjectName:    common.String(objectName),
		ContentLength: common.Int64(sizeBytes),
		ContentType:   common.String(contentType),
		PutObjectBody: io.NopCloser(body),
	})
	return err
}

func (s *OCIBlobStore) Get(ctx context.Context, bucketName, objectName string) (io.ReadCloser, int64, error) {
	client, namespace, err := s.resolve(ctx)
	if err != nil {
		return nil, 0, err
	}
	response, err := client.GetObject(ctx, objectstorage.GetObjectRequest{
		NamespaceName: common.String(namespace),
		BucketName:    common.String(bucketName),
		ObjectName:    common.String(objectName),
	})
	if err != nil {
		return nil, 0, err
	}
	size := int64(0)
	if response.ContentLength != nil {
		size = *response.ContentLength
	}
	return response.Content, size, nil
}

func (s *OCIBlobStore) Delete(ctx context.Context, bucketName, objectName string) error {
	client, namespace, err := s.resolve(ctx)
	if err != nil {
		return err
	}
	_, err = client.DeleteObject(ctx, objectstorage.DeleteObjectRequest{
		NamespaceName: common.String(namespace),
		BucketName:    common.String(bucketName),
		ObjectName:    common.String(objectName),
	})
	return err
}

func (s *OCIBlobStore) resolve(ctx context.Context) (ObjectStorageClient, string, error) {
	if s == nil || s.resolver == nil {
		return nil, "", fmt.Errorf("OCI cache blob store is not configured")
	}
	provider, ok, err := s.resolver.ResolveProvider(ctx)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("OCI cache blob store provider is not available")
	}
	client, err := s.clientFactory(provider)
	if err != nil {
		return nil, "", err
	}
	namespace, err := s.resolveNamespace(ctx, client)
	if err != nil {
		return nil, "", err
	}
	return client, namespace, nil
}

func (s *OCIBlobStore) resolveNamespace(ctx context.Context, client ObjectStorageClient) (string, error) {
	s.namespaceMu.Lock()
	defer s.namespaceMu.Unlock()
	if s.namespace != "" {
		return s.namespace, nil
	}
	response, err := client.GetNamespace(ctx, objectstorage.GetNamespaceRequest{})
	if err != nil {
		return "", err
	}
	if response.Value != nil {
		s.namespace = *response.Value
	}
	return s.namespace, nil
}
