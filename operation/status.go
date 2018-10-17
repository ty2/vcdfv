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
	if operationStatusFailure.OutputError != nil {
		outputMessage = operationStatusFailure.OutputError.Error()
	} else if operationStatusFailure.Error != nil {
		b, err := json.Marshal(struct {
			Error string `json:"error"`
		}{
			Error: operationStatusFailure.Error.Error(),
		})

		if err != nil {
			outputMessage = fmt.Sprintf(`{"error": "%s"}`, err.Error())
		}

		outputMessage = string(b)
	}

	return &ExecResult{
		Status:  ExecResultStatusFailure,
		Message: outputMessage,
	}, operationStatusFailure.Error
}
