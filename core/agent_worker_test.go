package core

import (
	"errors"
	"testing"

	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/core/common/enums"
)

func TestCheckExecutionFailed(t *testing.T) {
	tests := []struct {
		name   string
		result *config.Result
		err    error
		want   bool
	}{
		{
			name: "execution error fails",
			err:  errors.New("boom"),
			want: true,
		},
		{
			name: "nil result fails",
			want: true,
		},
		{
			name: "failed status fails",
			result: &config.Result{
				Status: enums.JobExecutionStatusFailed,
			},
			want: true,
		},
		{
			name: "success status passes",
			result: &config.Result{
				Status: enums.JobExecutionStatusSuccess,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkExecutionFailed(tt.result, tt.err)
			if got != tt.want {
				t.Fatalf("checkExecutionFailed()=%v, want %v", got, tt.want)
			}
		})
	}
}
