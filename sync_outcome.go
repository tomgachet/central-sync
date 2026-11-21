package main

const (
	exitSuccess        = 0
	exitPartialSuccess = 1
	exitFailure        = 2
)

// Each flag records whether any configured work produced that result.
type syncOutcome struct {
	succeeded bool
	failed    bool
}

func (outcome *syncOutcome) record(succeeded bool, err error) {
	outcome.succeeded = outcome.succeeded || succeeded
	outcome.failed = outcome.failed || err != nil
}

func (outcome syncOutcome) status() string {
	switch outcome.exitCode() {
	case exitSuccess:
		return "success"
	case exitPartialSuccess:
		return "partial_success"
	default:
		return "failed"
	}
}

func (outcome syncOutcome) exitCode() int {
	switch {
	case !outcome.succeeded:
		return exitFailure
	case outcome.failed:
		return exitPartialSuccess
	default:
		return exitSuccess
	}
}
