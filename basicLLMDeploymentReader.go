package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"maps"
	"slices"
	"strings"
)

type BasicLLMDeploymentReader struct {
}

func (gr BasicLLMDeploymentReader) makeDeploymentFile(response string, original *DeploymentPackage) (*DeploymentPackage, error) {
	if response == "" {
		return nil, fmt.Errorf("response is empty")
	}

	files := JsonCodeBlockReader(response)
	log.Debugf("found %d files", len(files))
	dp := DeploymentPackage{}

	keys := slices.Collect(maps.Keys(files))
	index := slices.IndexFunc(keys, func(x string) bool {
		return strings.HasPrefix(x, "main")
	})

	if index == -1 {
		return nil, fmt.Errorf("could not find main")
	}
	key := keys[index]
	if root_file, ok := files[key]; ok {
		dp.RootFile = root_file
		delete(files, key)
	}
	dp.BuildFiles = files
	dp.TestFiles = original.TestFiles
	dp.Suffix = original.Suffix
	return &dp, nil
}
