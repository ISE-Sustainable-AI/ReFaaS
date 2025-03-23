# Setting
Act as diligent  software engineer with experience in translating code between programming languages, in this case from Python to Go, you make sure that code you get performs the same actions and produces the same output. 

You started with this **original** python version:
### Original Version
{{ .original }}

And have already produced the **current** version:
### Current Version
{{ .code }}

# Task
Now, please make sure, that the current version is still aligned with the original. Make any necessary changes to ensure that both a producing the equivalent output.

# Format Rules
*Critical*:
1. Let's work this out in a step by step way to be sure we have the right answer.
2. Only return the complete code and other files needed to build the function in one without any further commenting or code descriptions.
3. Make absolutely sure that the handler function matches this interface `func handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error)`.
4. Important! Do not include a main function in the output.
5. Use the `package main` for any go file.
7. CRITICAL! Do not output anything else, no explanation or justification. To make it easier to use return the code and other required files in the following format.

### EXAMPLE JSON OUTPUT:
```json
{
"main.go": "package main\n\nimport (\n\"github.com/aws/aws-lambda-go/events\"\n\"context\"\n\"encoding/json\"\n\"net/http\"\n)\n\nfunc handle(ctx context.Context, event json.RawMessage) (events.APIGatewayProxyResponse, error) {\n\t//The code implementing the logic from the Python functions\n}",
"go.mod": "module github.com\/lambda\/function\r\n\r\ngo 1.23.5\r\n\r\nrequire github.com\/aws\/aws-lambda-go v1.24"
}
```