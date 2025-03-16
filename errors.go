package main

type TestingError struct {
	error
	error_code int
}

func (e TestingError) Error() string {
	return e.error.Error()
}

type CompilationError struct {
	error
}

func (e CompilationError) Error() string {
	return e.error.Error()
}

type LLMError struct {
	error
}

func (e LLMError) Error() string {
	return e.error.Error()
}
