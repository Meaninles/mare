package catalog

import (
	"context"

	cd2fs "mam/backend/internal/cd2/fs"
	"mam/backend/internal/credentials"
	"mam/backend/internal/store"
)

type ServiceOption interface {
	apply(*serviceOptions)
}

type serviceOptions struct {
	mediaConfig           MediaConfig
	credentialVault       *credentials.Vault
	autoQueueDerivedMedia bool
	autoQueueSearchJobs   bool
	searchBridge          SearchAIBridge
	cloud115UploadFactory cloud115UploadClientFactory
	cd2fsService          *cd2fs.Service
}

func (config MediaConfig) apply(options *serviceOptions) {
	options.mediaConfig = config
}

type credentialVaultOption struct {
	vault *credentials.Vault
}

func (option credentialVaultOption) apply(options *serviceOptions) {
	options.credentialVault = option.vault
}

func WithCredentialVault(vault *credentials.Vault) ServiceOption {
	return credentialVaultOption{vault: vault}
}

type autoQueueDerivedMediaOption struct {
	enabled bool
}

func (option autoQueueDerivedMediaOption) apply(options *serviceOptions) {
	options.autoQueueDerivedMedia = option.enabled
}

func WithAutoQueueDerivedMedia(enabled bool) ServiceOption {
	return autoQueueDerivedMediaOption{enabled: enabled}
}

type autoQueueSearchJobsOption struct {
	enabled bool
}

func (option autoQueueSearchJobsOption) apply(options *serviceOptions) {
	options.autoQueueSearchJobs = option.enabled
}

func WithAutoQueueSearchJobs(enabled bool) ServiceOption {
	return autoQueueSearchJobsOption{enabled: enabled}
}

type searchBridgeOption struct {
	bridge SearchAIBridge
}

func (option searchBridgeOption) apply(options *serviceOptions) {
	options.searchBridge = option.bridge
}

func WithSearchBridge(bridge SearchAIBridge) ServiceOption {
	return searchBridgeOption{bridge: bridge}
}

type cloud115UploadFactoryOption struct {
	factory cloud115UploadClientFactory
}

func (option cloud115UploadFactoryOption) apply(options *serviceOptions) {
	options.cloud115UploadFactory = option.factory
}

func WithCloud115UploadFactory(
	factory func(context.Context, store.StorageEndpoint) (cloud115UploadClient, cloud115UploadTarget, error),
) ServiceOption {
	return cloud115UploadFactoryOption{factory: factory}
}

type cd2fsServiceOption struct {
	service *cd2fs.Service
}

func (option cd2fsServiceOption) apply(options *serviceOptions) {
	options.cd2fsService = option.service
}

func WithCD2FSService(service *cd2fs.Service) ServiceOption {
	return cd2fsServiceOption{service: service}
}
