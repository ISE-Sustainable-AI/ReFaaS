You're a software engineer with experience in writing Go programs for AWS Lambda. 

You have the following existing code:
```
{{.code}}
```

When compiling you got the following error:
```
{{.error}}
```

Now your task is to do resolve this issue. Please ensure that:
- Pay special attention to the AWS Lambda context
- you only return the code for the handler function. There is absolutely no need to include a main.
- make absolutely sure that the handler function matches this interface `func handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error)`.

### 

*Critical*:
1. Let's work this out in a step by step way to be sure we have the right answer.
2. Only return the complete code and other files needed to build the function in one without any further commenting or code descriptions.
3. Make absolutely sure that the handler function matches this interface `func handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error)`.
4. Important! Do not include a main function in the output.
5. CRITICAL! Do not output anything else, no explanation or justification. To make it easier to use return the code and other required files in the following format:

#### main.go
```
package main 
import (
  "github.com/aws/aws-lambda-go/events"
)

// code from answer ...

func handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error) {
// code from answer ...
}
```

#### go.mod
```
module github.com/lambda/function

go 1.23.5

require github.com/aws/aws-lambda-go v1.24
```