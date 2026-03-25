package catalog

import "mam/backend/internal/credentials"

type ServiceOption interface {
	apply(*serviceOptions)
}

type serviceOptions struct {
	mediaConfig           MediaConfig
	credentialVault       *credentials.Vault
	autoQueueDerivedMedia bool
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
