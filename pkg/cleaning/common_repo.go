package cleaning

import (
	"strings"

	"github.com/flant/logboek"

	"github.com/flant/werf/pkg/docker_registry"
	"github.com/flant/werf/pkg/image"
)

type CommonRepoOptions struct {
	StagesStorage     string
	ImagesRepoManager ImagesRepoManager
	ImagesNames       []string
	DryRun            bool
}

type ImagesRepoManager interface {
	ImagesRepo() string
	ImageRepo(imageName string) string
	ImageRepoWithTag(imageName, tag string) string
	IsMonorep() bool
}

func repoImages(options CommonRepoOptions) (repoImages []docker_registry.RepoImage, err error) {
	repoImagesByImageName, err := repoImagesByImageName(options)
	if err != nil {
		return nil, err
	}

	for _, imageRepoImages := range repoImagesByImageName {
		repoImages = append(repoImages, imageRepoImages...)
	}

	return
}

func repoImagesByImageName(options CommonRepoOptions) (repoImagesByImageName map[string][]docker_registry.RepoImage, err error) {
	if err := logboek.LogProcessInline("Getting repo images", logboek.LogProcessInlineOptions{}, func() error {
		if options.ImagesRepoManager.IsMonorep() {
			repoImagesByImageName, err = monorepRepoImages(options)
		} else {
			repoImagesByImageName, err = multirepRepoImages(options)
		}

		return err
	}); err != nil {
		return nil, err
	}

	logboek.LogOptionalLn()

	return repoImagesByImageName, nil
}

func monorepRepoImages(options CommonRepoOptions) (map[string][]docker_registry.RepoImage, error) {
	repoImagesByImageName := map[string][]docker_registry.RepoImage{}
	for _, imageName := range options.ImagesNames {
		repoImagesByImageName[imageName] = []docker_registry.RepoImage{}
	}

	repoImages, err := docker_registry.ImagesByWerfImageLabel(options.ImagesRepoManager.ImagesRepo(), "true")
	if err != nil {
		return nil, err
	}

loop:
	for _, repoImage := range repoImages {
		for _, imageName := range options.ImagesNames {
			labels, err := repoImageLabels(repoImage)
			if err != nil {
				return nil, err
			}

			repoImageMetaName, ok := labels[image.WerfImageNameLabel]
			if !ok {
				continue
			}

			if repoImageMetaName == imageName {
				repoImagesByImageName[imageName] = append(repoImagesByImageName[imageName], repoImage)
				continue loop
			}
		}
	}

	return repoImagesByImageName, nil
}

func multirepRepoImages(options CommonRepoOptions) (map[string][]docker_registry.RepoImage, error) {
	repoImagesByImageName := map[string][]docker_registry.RepoImage{}
	for _, imageName := range options.ImagesNames {
		repoImagesByImageName[imageName] = []docker_registry.RepoImage{}

		imageRepo := options.ImagesRepoManager.ImageRepo(imageName)
		images, err := docker_registry.ImagesByWerfImageLabel(imageRepo, "true")
		if err != nil {
			return nil, err
		}

		repoImagesByImageName[imageName] = append(repoImagesByImageName[imageName], images...)
	}

	return repoImagesByImageName, nil
}

func repoImageStagesImages(options CommonRepoOptions) ([]docker_registry.RepoImage, error) {
	return docker_registry.ImagesByWerfImageLabel(options.StagesStorage, "false")
}

func repoImagesRemove(images []docker_registry.RepoImage, options CommonRepoOptions) error {
	isGCR, err := docker_registry.IsGCR(options.ImagesRepoManager.ImagesRepo())
	if err != nil {
		return err
	}

	for _, image := range images {
		if isGCR {
			if err := GCRImageRemove(image, options); err != nil {
				return err
			}
		} else {
			if err := repoImageRemove(image, options); err != nil {
				return err
			}
		}
	}

	return nil
}

func GCRImageRemove(image docker_registry.RepoImage, options CommonRepoOptions) error {
	reference := strings.Join([]string{image.Repository, image.Tag}, ":")
	if err := repoReferenceRemove(reference, options); err != nil {
		return err
	}

	return nil
}

func repoImageRemove(image docker_registry.RepoImage, options CommonRepoOptions) error {
	digest, err := image.Digest()
	if err != nil {
		return err
	}

	reference := strings.Join([]string{image.Repository, digest.String()}, "@")
	if err := repoReferenceRemove(reference, options); err != nil {
		return err
	}

	logboek.LogInfoF("  tag: %s\n", image.Tag)
	logboek.LogOptionalLn()

	return nil
}

func repoReferenceRemove(reference string, options CommonRepoOptions) error {
	logboek.LogLn(reference)
	if !options.DryRun {
		err := docker_registry.ImageDelete(reference)
		if err != nil {
			return err
		}
	}

	return nil
}

func exceptRepoImages(repoImages []docker_registry.RepoImage, repoImagesToExclude ...docker_registry.RepoImage) []docker_registry.RepoImage {
	var newRepoImages []docker_registry.RepoImage

Loop:
	for _, repoImage := range repoImages {
		reference := strings.Join([]string{repoImage.Repository, repoImage.Tag}, ":")
		for _, repoImageToExclude := range repoImagesToExclude {
			referenceToExclude := strings.Join([]string{repoImageToExclude.Repository, repoImageToExclude.Tag}, ":")
			if reference == referenceToExclude {
				continue Loop
			}
		}

		newRepoImages = append(newRepoImages, repoImage)
	}

	return newRepoImages
}
