package main

import "testing"

func Test_process(t *testing.T) {
	type args struct {
		target string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Test successful execution",
			args: args{
				target: "https://github.com/features",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := process(tt.args.target); (err != nil) != tt.wantErr {
				t.Errorf("process() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
