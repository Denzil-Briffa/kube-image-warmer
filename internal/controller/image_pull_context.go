package controller

import (
	"slices"
	"strings"

	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

type imagePullContextKey struct {
	image            string
	namespace        string
	imagePullSecrets string
}

func deduplicateImagePullContexts(
	discoveredImages []discovery.DiscoveredImage,
) []discovery.DiscoveredImage {
	uniqueImages := make(
		[]discovery.DiscoveredImage,
		0,
		len(discoveredImages),
	)
	seenContexts := make(
		map[imagePullContextKey]bool,
		len(discoveredImages),
	)

	for _, discoveredImage := range discoveredImages {
		secretNames := make(
			[]string,
			len(discoveredImage.ImagePullSecrets),
		)

		for i := range discoveredImage.ImagePullSecrets {
			secretNames[i] = discoveredImage.ImagePullSecrets[i].Name
		}

		slices.Sort(secretNames)

		key := imagePullContextKey{
			image:            discoveredImage.Image,
			namespace:        discoveredImage.Namespace,
			imagePullSecrets: strings.Join(secretNames, "\x00"),
		}

		if seenContexts[key] {
			continue
		}

		seenContexts[key] = true
		uniqueImages = append(uniqueImages, discoveredImage)
	}
	return uniqueImages
}
