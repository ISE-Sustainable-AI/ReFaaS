package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"time"
)

// NewPipeline initializes a new pipeline
func NewPipeline(firstTask *ConversionTask) *Pipeline {
	return &Pipeline{FirstTask: firstTask}
}

// Execute runs the pipeline
func (p *Pipeline) Execute(runner *PipelineRunner, req *ConversionRequest) error {
	req.Metrics.StartTime = time.Now()
	defer func() {
		req.Metrics.EndTime = time.Now()
		req.Metrics.TotalTime = req.Metrics.EndTime.Sub(req.Metrics.StartTime)
	}()

	return p.executeTask(runner, req, p.FirstTask)
}

// executeTask runs an individual task with retry logic and failure handling
func (p *Pipeline) executeTask(runner *PipelineRunner, req *ConversionRequest, task *ConversionTask) error {
	if task == nil {
		return nil
	}

	req.Metrics.Tasks += 1

	if task.CanApply != nil {
		if applyErr := task.CanApply.Apply(runner, req); applyErr != nil {
			return fmt.Errorf("task %s precondition failed - %v", task.ID, applyErr)
		}
	}

	var err error
	for ; task.RetryCount < task.MaxRetryCount; task.RetryCount++ {
		err = task.Execute.Apply(runner, req)
		if err == nil {
			log.Debugf("task %s executed successfully", task.ID)
			break
		}

		if task.RetryCount+1 < task.MaxRetryCount {
			log.Debugf("task %s retry failed (%+v), retrying...", task.ID, err)

			if task.OnFailure != nil {
				req.err = err
				log.Debugf("atempting to recover task %s before retring", task.ID)
				err = p.executeTask(runner, req, task.OnFailure)
				if err == nil {
					// Continue to next retry attempt of TaskB without exceeding max retries
					log.Debugf("Retrying failed task %s after recovery", task.ID)
					continue
				} else {
					log.Debugf("Recovery failed.")
					break
				}
			}
			time.Sleep(task.RetryDelay)
		}
	}

	if err != nil {
		log.Debugf("task %s failed.", task.ID)
		req.err = err
		return err
	}

	if task.Validation != nil {
		log.Debugf("performing validation task %s", task.ID)
		err = task.Validation.Apply(runner, req)
		if err != nil {
			log.Debugf("task validation for %s failed.", task.ID)
			req.err = err
			return err
		}
	}

	// Execute next tasks
	for _, next := range task.Next {
		if err := p.executeTask(runner, req, next); err != nil {
			req.err = err
			return err
		}
	}

	return nil
}
