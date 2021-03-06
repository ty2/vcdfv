package operation

import (
	"encoding/json"
	"fmt"
)

type StatusFailure struct {
	Error       error
	OutputError error
}

func (operationStatusFailure *StatusFailure) Exec() (*ExecResult, error) {
	var outputMessage string

	// use output as OutputError as first priority
	// use output as Error as second priority
	if operationStatusFailure.OutputError != nil {
		outputMessage = operationStatusFailure.OutputError.Error()
	} else if operationStatusFailure.Error != nil {
		b, err := json.Marshal(struct {
			Error string `json:"error"`
		}{
			Error: operationStatusFailure.Error.Error(),
		})

		if err != nil {
			// json unmarshal error, use DIY output
			outputMessage = fmt.Sprintf(`{"error": "%s"}`, err.Error())
		}

		outputMessage = string(b)
	}

	return &ExecResult{
		Status:  ExecResultStatusFailure,
		Message: outputMessage,
	}, operationStatusFailure.Error
}

type StatusSuccess struct {
	JsonMessageStruct interface{}
}

func (statusSuccess *StatusSuccess) Exec() (*ExecResult, error) {
	message, err := json.Marshal(statusSuccess.JsonMessageStruct)

	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	return &ExecResult{
		Status:  ExecResultStatusSuccess,
		Message: string(message),
	}, nil
}
