import json

def simple_addition(event, context):
    num1 = event.get('num1', 0)
    num2 = event.get('num2', 0)
    result = num1 + num2
    return {
        'statusCode': 200,
        'body': json.dumps({'result': result})
    }

def lambda_handler(event, context):
    return simple_addition(event, context)