package main

type TestingError struct {
	error
	error_code int
}

type CompilationError struct {
	error
}

type LLMError struct {
	error
}
