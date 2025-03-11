You're a software engineer with experience in translating code between programming languages, in this case from Python to Go. You have a understanding of both languages especially in the context of AWS Lambda.

Your task is to translate the following Python code into Go. Please ensure that:

- the functionality remains the same
- the import format is consistent with Go standards-
- Pay special attention to the AWS Lambda context
- you only return the code for the handler function no need to include a main
- make absolutely sure that the handler function matches this interface `func handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error)`.

Following are two aws imports that have to be included for the event handling to work. Make sure that they are present beside the other imports needed.

###

import (
"github.com/aws/aws-lambda-go/events"
)

###

The following example shows a working implementation of Input Event handling in go. It is of highest priority that you implement the event handling in this way.

Go input handling example:

###
```
package main

import (
 "github.com/aws/aws-lambda-go/events"
)


type requestBodyExample struct {
    Num1      float64 `json:"num1"`
    Num2      float64 `json:"num2"`
}

func handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error) {
    var request requestBodyExample
    if err := json.Unmarshal(event, &request); err != nil {
        log.Printf("Failed to unmarshal event: %v", err)
        return events.APIGatewayProxyResponse{
            StatusCode: http.StatusBadRequest,
            Body:       "Invalid input",
        }, nil
    }
    return events.APIGatewayProxyResponse{
            StatusCode: http.StatusOK,
            Body:       fmt.Sprintf("%d",reqeust.Num1+request.Num2),
    }, nil
}
```
###
Now follows the python code that should be translated to go:

```
{{ .code }}
```

### 

*Critical*:
1. Let's work this out in a step by step way to be sure we have the right answer.
2. Only return the complete code and other files needed to build the function in one without any further commenting or code descriptions.
3. Make absolutely sure that the handler function matches this interfaceL `func handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error)`.
4. Important! Do not include a main function in the output.
5. Use the `package main` for any go file.
6. CRITICAL! Do not output anything else, no explanation or justification. To make it easier to use return the code and other required files in the following format.
###
EXAMPLE JSON OUTPUT:
{
  "main.go": "package main \r\nimport (\r\n  \"github.com\/aws\/aws-lambda-go\/events\"\r\n)\r\n\r\n\/\/ code from answer ...\r\n\r\nfunc handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error) {\r\n\/\/ code from answer ...\r\n}",
  "go.mod": "module github.com\/lambda\/function\r\n\r\ngo 1.23.5\r\n\r\nrequire github.com\/aws\/aws-lambda-go v1.24"
}
