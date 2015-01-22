package models

import (
	"encoding/json"
	"net/url"
	"regexp"
)

//go:generate stringer -type=TaskState
type TaskState int

const (
	TaskStateInvalid TaskState = iota
	TaskStatePending
	TaskStateRunning
	TaskStateCompleted
	TaskStateResolving
)

const maximumAnnotationLength = 10 * 1024

var taskGuidPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Task struct {
	TaskGuid             string                `json:"task_guid"`
	Domain               string                `json:"domain"`
	RootFSPath           string                `json:"root_fs"`
	Stack                string                `json:"stack"`
	EnvironmentVariables []EnvironmentVariable `json:"env,omitempty"`
	Action               Action                `json:"-"`
	MemoryMB             int                   `json:"memory_mb"`
	Privileged           bool                  `json:"privileged"`
	DiskMB               int                   `json:"disk_mb"`
	CPUWeight            uint                  `json:"cpu_weight"`
	LogSource            string                `json:"log_source"`
	LogGuid              string                `json:"log_guid"`
	CreatedAt            int64                 `json:"created_at"` //  the number of nanoseconds elapsed since January 1, 1970 UTC
	UpdatedAt            int64                 `json:"updated_at"`
	FirstCompletedAt     int64                 `json:"first_completed_at"`
	ResultFile           string                `json:"result_file"`

	State TaskState `json:"state"`

	CellID string `json:"cell_id"`

	Result        string `json:"result"`
	Failed        bool   `json:"failed"`
	FailureReason string `json:"failure_reason"`

	CompletionCallbackURL *url.URL `json:"completion_callback_url,omitempty"`
	Annotation            string   `json:"annotation,omitempty"`

	EgressRules []SecurityGroupRule `json:"egress_rules,omitempty"`
}

type InnerTask Task

type mTask struct {
	ActionRaw *json.RawMessage `json:"action,omitempty"`
	*InnerTask
}

type StagingResult struct {
	BuildpackKey         string            `json:"buildpack_key,omitempty"`
	DetectedBuildpack    string            `json:"detected_buildpack"`
	ExecutionMetadata    string            `json:"execution_metadata"`
	DetectedStartCommand map[string]string `json:"detected_start_command"`
}

type StagingDockerResult struct {
	ExecutionMetadata    string            `json:"execution_metadata"`
	DetectedStartCommand map[string]string `json:"detected_start_command"`
}

type StagingTaskAnnotation struct {
	AppId  string `json:"app_id"`
	TaskId string `json:"task_id"`
}

func (t *Task) UnmarshalJSON(payload []byte) error {
	mtask := mTask{InnerTask: (*InnerTask)(t)}
	err := json.Unmarshal(payload, &mtask)
	if err != nil {
		return err
	}

	var a Action
	if mtask.ActionRaw == nil {
		a = nil
	} else {
		a, err = UnmarshalAction(*mtask.ActionRaw)
		if err != nil {
			return err
		}
	}
	t.Action = a

	return nil
}

func (task Task) Validate() error {
	var validationError ValidationError

	if task.Domain == "" {
		validationError = validationError.Append(ErrInvalidField{"domain"})
	}

	if !taskGuidPattern.MatchString(task.TaskGuid) {
		validationError = validationError.Append(ErrInvalidField{"task_guid"})
	}

	if task.Stack == "" {
		validationError = validationError.Append(ErrInvalidField{"stack"})
	}

	if task.Action == nil {
		validationError = validationError.Append(ErrInvalidActionType)
	} else {
		err := task.Action.Validate()
		if err != nil {
			validationError = validationError.Append(err)
		}
	}

	if task.CPUWeight > 100 {
		validationError = validationError.Append(ErrInvalidField{"cpu_weight"})
	}

	if len(task.Annotation) > maximumAnnotationLength {
		validationError = validationError.Append(ErrInvalidField{"annotation"})
	}

	for _, rule := range task.EgressRules {
		err := rule.Validate()
		if err != nil {
			validationError = validationError.Append(ErrInvalidField{"egress_rules"})
		}
	}

	if !validationError.Empty() {
		return validationError
	}

	return nil
}

func (task Task) MarshalJSON() ([]byte, error) {
	actionRaw, err := MarshalAction(task.Action)
	if err != nil {
		return nil, err
	}

	rm := json.RawMessage(actionRaw)
	innerTask := InnerTask(task)

	mtask := &mTask{
		ActionRaw: &rm,
		InnerTask: &innerTask,
	}

	return json.Marshal(mtask)
}
