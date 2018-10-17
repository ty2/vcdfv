package operation

type Init struct{}

func (init *Init) Exec() (*ExecResult, error) {
	return &ExecResult{
		Status:  ExecResultStatusSuccess,
		Message: "Initial success",
		Capabilities: &ExecCapabilities{
			Attach: false,
		},
	}, nil
}
