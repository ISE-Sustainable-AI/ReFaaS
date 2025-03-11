package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (cc *CodeConverter) runTest(ctx context.Context, code *DeploymentPackage) (bool, error) {
	dir, err := os.MkdirTemp("", "fn_lmm")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(dir)

	code.BuildFiles["handler.go"] = string(cc.TestHandler)
	//Build testable version
	err = cc.build(ctx, code, dir)
	if err != nil {
		log.Debugf("failed to build: %s", err.Error())
		return false, CompilationError{err}
	}

	start_time := time.Now()
	err_cnt := 0

	for test_name, test := range code.TestFiles {
		testfile := TestFile{}
		code.Metrics.TestCases[test_name] = false
		err := json.Unmarshal([]byte(test), &testfile)
		if err != nil {
			log.Debugf("failed to read test %s: %+v", test_name, err)
			err_cnt++
			continue
		}

		success, err := cc.doTest(ctx, dir, &testfile)
		if err != nil {
			err_cnt++
			log.Debugf("test %s failed: %v", test_name, err)
			continue
		}
		if !success {
			err_cnt++
			log.Debugf("test %s failed: %v", test_name, err)
		}
		code.Metrics.TestCases[test_name] = true
	}
	code.Metrics.TestTime = time.Since(start_time)
	code.Metrics.TestError = err_cnt
	if err_cnt != 0 {
		return false, TestingError{fmt.Errorf("%d tests failed", err_cnt), err_cnt}
	}

	return true, nil
}

func (cc *CodeConverter) build(ctx context.Context, code *DeploymentPackage, dir string) error {
	build_start_time := time.Now()
	for i := 0; i < cc.buildAttempts; i++ {
		out, err := cc.doBuild(ctx, code, dir)
		if err == nil {
			break //we are done
		} else if cc.buildRePrompting && i < cc.buildAttempts-1 {
			//TODO: repromt and rebuild out.
			new_code, metrics, err := cc.repromptOnBuildError(ctx, code, out)
			if err != nil {
				log.Debugf("failed to repromt: %v", err)
				code.Metrics.AddMetric(metrics)
				return err
			}
			code = new_code
		} else {
			code.Metrics.BuildTime = time.Since(build_start_time)
			log.Debugf("failed to build")
			return err
		}
	}
	code.Metrics.BuildTime = time.Since(build_start_time)
	return nil
}

func (cc *CodeConverter) doBuild(ctx context.Context, code *DeploymentPackage, dir string) (string, error) {
	err := cc.prepareBuildFolder(dir, code)
	if err != nil {
		log.Debugf("failed to prepare build folder: %s", err.Error())
		return "", err
	}
	for _, cmd := range code.BuildCmd {
		out, err := cc.runBuildCommands(ctx, dir, cmd)
		if err != nil {
			log.Debugf("failed to run build commands: %+v", err)
			if strings.Contains(err.Error(), " unknown revision") {
				//atempt to remove go.mod to fix the issue
				delete(code.BuildFiles, "go.mod")
				code.BuildCmd = []string{
					"go mod init example.com",
					"go mod tidy",
					"go build -o fn .",
				}
				out, err := cc.runBuildCommands(ctx, dir, cmd)
				return out, err
			} else {
				code.Metrics.BuildError += 1
				return out, err
			}

		}
	}
	return "", nil
}

func (cc *CodeConverter) prepareBuildFolder(dir string, code *DeploymentPackage) error {
	//TODO: delete
	writeToDir := func(fname, code string) error {
		fpath := filepath.Join(dir, fname)
		if _, err := os.Stat(fpath); err == nil {
			err := os.Remove(fpath)
			if err != nil {
				return err
			}
		}

		fs, err := os.OpenFile(fpath, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", fname, err)
		}
		defer fs.Close()
		_, err = fs.Write([]byte(code))
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", fname, err)
		}
		return nil
	}
	err := writeToDir("main.go", code.RootFile)
	if err != nil {
		return err
	}
	for fname, file := range code.BuildFiles {
		err = writeToDir(fname, file)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cc *CodeConverter) runBuildCommands(ctx context.Context, dir, build_cmd string) (string, error) {
	cmds := strings.Split(build_cmd, " ")

	cmd := exec.CommandContext(ctx, cmds[0], cmds[1:]...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()

	if err != nil {
		return stdout.String(), fmt.Errorf("failed to build. %s \n\n %+v", stdout.String(), err)
	}
	return stdout.String(), nil
}
