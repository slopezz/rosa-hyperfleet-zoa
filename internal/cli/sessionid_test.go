package cli

import (
	"testing"
)

func TestFormatSessionID(t *testing.T) {
	tests := []struct {
		name       string
		deployment string
		rawID      string
		want       string
	}{
		{
			name:       "When given a normal deployment and ID it should join them",
			deployment: "us-east-1",
			rawID:      "sess-abc123",
			want:       "us-east-1/sess-abc123",
		},
		{
			name:       "When given an ephemeral deployment it should include the full name",
			deployment: "us-east-1-eph-f8d5483c",
			rawID:      "sess-xyz789",
			want:       "us-east-1-eph-f8d5483c/sess-xyz789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSessionID(tt.deployment, tt.rawID)
			if got != tt.want {
				t.Errorf("FormatSessionID(%q, %q) = %q, want %q", tt.deployment, tt.rawID, got, tt.want)
			}
		})
	}
}

func TestParseSessionID(t *testing.T) {
	tests := []struct {
		name           string
		compound       string
		wantDeployment string
		wantRawID      string
		wantErr        bool
	}{
		{
			name:           "When given a valid compound ID it should split correctly",
			compound:       "us-east-1/sess-abc123",
			wantDeployment: "us-east-1",
			wantRawID:      "sess-abc123",
		},
		{
			name:           "When given an ephemeral compound ID it should split correctly",
			compound:       "us-east-1-eph-f8d5483c/sess-xyz789",
			wantDeployment: "us-east-1-eph-f8d5483c",
			wantRawID:      "sess-xyz789",
		},
		{
			name:     "When given a bare session ID it should return an error",
			compound: "sess-abc123",
			wantErr:  true,
		},
		{
			name:     "When given an empty string it should return an error",
			compound: "",
			wantErr:  true,
		},
		{
			name:     "When given only a separator it should return an error",
			compound: "/",
			wantErr:  true,
		},
		{
			name:     "When given a trailing separator it should return an error",
			compound: "us-east-1/",
			wantErr:  true,
		},
		{
			name:     "When given a leading separator it should return an error",
			compound: "/sess-abc123",
			wantErr:  true,
		},
		{
			name:           "When given extra slashes in the raw ID it should keep them",
			compound:       "us-east-1/task/abc123",
			wantDeployment: "us-east-1",
			wantRawID:      "task/abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment, rawID, err := ParseSessionID(tt.compound)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseSessionID(%q) expected error, got deployment=%q rawID=%q", tt.compound, deployment, rawID)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseSessionID(%q) unexpected error: %v", tt.compound, err)
				return
			}
			if deployment != tt.wantDeployment {
				t.Errorf("ParseSessionID(%q) deployment = %q, want %q", tt.compound, deployment, tt.wantDeployment)
			}
			if rawID != tt.wantRawID {
				t.Errorf("ParseSessionID(%q) rawID = %q, want %q", tt.compound, rawID, tt.wantRawID)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		deployment string
		rawID      string
	}{
		{"us-east-1", "sess-abc123"},
		{"us-east-1-eph-f8d5483c", "task-xyz"},
	}

	for _, tt := range tests {
		t.Run("When round-tripping "+tt.deployment+" it should preserve both parts", func(t *testing.T) {
			compound := FormatSessionID(tt.deployment, tt.rawID)
			gotDep, gotID, err := ParseSessionID(compound)
			if err != nil {
				t.Fatalf("ParseSessionID(%q) unexpected error: %v", compound, err)
			}
			if gotDep != tt.deployment {
				t.Errorf("deployment = %q, want %q", gotDep, tt.deployment)
			}
			if gotID != tt.rawID {
				t.Errorf("rawID = %q, want %q", gotID, tt.rawID)
			}
		})
	}
}
