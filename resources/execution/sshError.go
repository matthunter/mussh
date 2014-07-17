package execution

type sshError struct {
	description string
	stack       error
}

func (exe sshError) Error() string {
	return exe.description + "\n\n" + exe.stack.Error()
}
