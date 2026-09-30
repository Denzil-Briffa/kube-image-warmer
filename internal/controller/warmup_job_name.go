package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

func warmupJobName(
	policy *cachev1alpha1.ImageWarmupPolicy,
	runID string,
	targetNode *corev1.Node,
	image discovery.DiscoveredImage,
) string {
	secretNames := make(
		[]string,
		len(image.ImagePullSecrets),
	)

	for i := range image.ImagePullSecrets {
		secretNames[i] =
			image.ImagePullSecrets[i].Name
	}

	slices.Sort(secretNames)

	identity := strings.Join(
		[]string{
			string(policy.UID),
			runID,
			string(targetNode.UID),
			image.Image,
			image.Namespace,
			strings.Join(secretNames, "\x00"),
		},
		"\x00",
	)

	hash := sha256.Sum256([]byte(identity))
	shortHash := hex.EncodeToString(hash[:])[:20]

	return "image-warmup-" + shortHash
}
