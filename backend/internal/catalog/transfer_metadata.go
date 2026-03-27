package catalog

import (
	"encoding/json"
	"strings"

	"mam/backend/internal/store"
)

func readTransferItemMetadata(item store.TransferTaskItem) transferItemMetadata {
	return parseTransferItemMetadata(item.MetadataJSON)
}

func writeTransferItemMetadata(item *store.TransferTaskItem, metadata transferItemMetadata) {
	if item == nil {
		return
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return
	}
	item.MetadataJSON = string(encoded)
}

func updateTransferItemMetadata(item *store.TransferTaskItem, mutate func(*transferItemMetadata)) transferItemMetadata {
	metadata := readTransferItemMetadata(*item)
	if mutate != nil {
		mutate(&metadata)
	}
	writeTransferItemMetadata(item, metadata)
	return metadata
}

func transferItemExternalTaskID(item store.TransferTaskItem) string {
	metadata := readTransferItemMetadata(item)
	switch strings.TrimSpace(metadata.EngineKind) {
	case transferEngineKindAList:
		if metadata.AList != nil && strings.TrimSpace(metadata.AList.TaskID) != "" {
			return strings.TrimSpace(metadata.AList.TaskID)
		}
	case transferEngineKindAria2:
		if metadata.Aria2 != nil && strings.TrimSpace(metadata.Aria2.GID) != "" {
			return strings.TrimSpace(metadata.Aria2.GID)
		}
	case transferEngineKindCloud115:
		if metadata.Cloud115 != nil && strings.TrimSpace(metadata.Cloud115.UploadID) != "" {
			return strings.TrimSpace(metadata.Cloud115.UploadID)
		}
	}
	if metadata.AList != nil && strings.TrimSpace(metadata.AList.TaskID) != "" {
		return strings.TrimSpace(metadata.AList.TaskID)
	}
	if metadata.Aria2 != nil && strings.TrimSpace(metadata.Aria2.GID) != "" {
		return strings.TrimSpace(metadata.Aria2.GID)
	}
	if metadata.Cloud115 != nil && strings.TrimSpace(metadata.Cloud115.UploadID) != "" {
		return strings.TrimSpace(metadata.Cloud115.UploadID)
	}
	return ""
}

func transferItemExternalStatus(item store.TransferTaskItem) string {
	metadata := readTransferItemMetadata(item)
	switch strings.TrimSpace(metadata.EngineKind) {
	case transferEngineKindAList:
		if metadata.AList != nil && strings.TrimSpace(metadata.AList.TaskStatus) != "" {
			return strings.TrimSpace(metadata.AList.TaskStatus)
		}
	case transferEngineKindAria2:
		if metadata.Aria2 != nil && strings.TrimSpace(metadata.Aria2.Status) != "" {
			return strings.TrimSpace(metadata.Aria2.Status)
		}
	case transferEngineKindCloud115:
		if metadata.Cloud115 != nil && strings.TrimSpace(metadata.Cloud115.Status) != "" {
			return strings.TrimSpace(metadata.Cloud115.Status)
		}
	}
	if metadata.AList != nil && strings.TrimSpace(metadata.AList.TaskStatus) != "" {
		return strings.TrimSpace(metadata.AList.TaskStatus)
	}
	if metadata.Aria2 != nil && strings.TrimSpace(metadata.Aria2.Status) != "" {
		return strings.TrimSpace(metadata.Aria2.Status)
	}
	if metadata.Cloud115 != nil && strings.TrimSpace(metadata.Cloud115.Status) != "" {
		return strings.TrimSpace(metadata.Cloud115.Status)
	}
	return ""
}
