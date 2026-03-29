package proto

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"regexp"
	"strings"

	cd2pb "mam/backend/internal/cd2/pb"

	"google.golang.org/protobuf/proto"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
)

const (
	SourceURL   = "https://www.clouddrive2.com/api/clouddrive.proto"
	ServiceName = "clouddrive.CloudDriveFileSrv"
)

var declaredVersionPattern = regexp.MustCompile(`option\s+\(version\)\s*=\s*"([^"]+)"`)

//go:embed clouddrive.proto
var rawSource string

func Source() string {
	return rawSource
}

func SourceSHA256() string {
	sum := sha256.Sum256([]byte(rawSource))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

func DeclaredVersion() string {
	match := declaredVersionPattern.FindStringSubmatch(rawSource)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func DescriptorVersion() string {
	options, ok := cd2pb.File_clouddrive_proto.Options().(*descriptorpb.FileOptions)
	if !ok || options == nil {
		return ""
	}
	if !proto.HasExtension(options, cd2pb.E_Version) {
		return ""
	}

	value := proto.GetExtension(options, cd2pb.E_Version)

	version, ok := value.(string)
	if ok {
		return strings.TrimSpace(version)
	}

	versionPointer, ok := value.(*string)
	if ok && versionPointer != nil {
		return strings.TrimSpace(*versionPointer)
	}

	return ""
}
