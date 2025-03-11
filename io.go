package main

import (
	"archive/zip"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"strings"
)

func (cc *CodeConverter) ReadDeploymentPackageFromFile(sourceFile string) (*DeploymentPackage, error) {
	fs, err := os.OpenFile(sourceFile, os.O_RDONLY, 0666)
	if err != nil {
		return nil, err
	}
	stat, err := fs.Stat()
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	dp, err := cc.ReadDeploymentPackageFromReader(fs, stat.Size())

	if err != nil {
		return nil, err
	}
	return dp, nil
}

func (cc *CodeConverter) ReadDeploymentPackageFromReader(reader io.ReaderAt, size int64) (*DeploymentPackage, error) {
	dp := DeploymentPackage{
		RootFile:  "",
		TestFiles: make(map[string]string),
		Metrics: &Metrics{
			TestCases: make(map[string]bool),
		},
	}
	zipfs, err := zip.NewReader(reader, size)
	if err != nil {
		return nil, err
	}
	for _, file := range zipfs.File {
		if strings.HasSuffix(file.Name, "."+cc.SourceSuffix) {
			fileReader, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer fileReader.Close()
			rootFile, err := io.ReadAll(fileReader)
			if err != nil {
				return nil, err
			}
			dp.RootFile = string(rootFile)
		} else if strings.HasPrefix(file.Name, "test/") {
			if file.FileInfo().IsDir() {
				continue
			}
			fileReader, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer fileReader.Close()
			testFile, err := io.ReadAll(fileReader)
			if err != nil {
				return nil, err
			}
			dp.TestFiles[file.Name] = string(testFile)
		}
	}
	return &dp, err
}

func (cc *CodeConverter) WriteDeploymentPackage(writer io.Writer, dp *DeploymentPackage) error {
	zw := zip.NewWriter(writer)

	writeFile := func(file string, content string) error {
		fp, err := zw.Create(file)
		if err != nil {
			return err
		}
		written, err := fp.Write([]byte(content))
		if err != nil {
			log.Debugf("Failed to write to %s: %s", file, err)
			return err
		}
		log.Debugf("Written %s [%d] bytes", file, written)
		return nil
	}

	err := writeFile("main.go", dp.RootFile)
	if err != nil {
		return err
	}
	for name, file := range dp.TestFiles {
		err := writeFile(name, file)
		if err != nil {
			return err
		}
	}

	for name, file := range dp.BuildFiles {
		err := writeFile(name, file)
		if err != nil {
			return err
		}
	}

	if len(dp.BuildCmd) > 0 {
		var builder strings.Builder
		builder.WriteString("#! /bin/sh\n\n")
		for _, line := range dp.BuildCmd {
			builder.WriteString(line)
			builder.WriteString("\n")
		}

		err := writeFile("build.sh", builder.String())
		if err != nil {
			return err
		}
	}
	err = zw.Close()
	return err
}
