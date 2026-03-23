package catalog

import "testing"

func TestNormalizeLogicalPathKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		rootPath     string
		physicalPath string
		expected     string
	}{
		{
			name:         "local windows path",
			rootPath:     `D:\Media`,
			physicalPath: `D:\Media\Projects\Travel\Sunset.JPG`,
			expected:     "projects/travel/sunset.jpg",
		},
		{
			name:         "qnap smb path",
			rootPath:     `\\qnap\share\Media`,
			physicalPath: `\\qnap\share\Media\Projects\Travel\Sunset.JPG`,
			expected:     "projects/travel/sunset.jpg",
		},
		{
			name:         "cloud115 relative path",
			rootPath:     "0",
			physicalPath: `Projects/Travel/./RAW/../Sunset.JPG`,
			expected:     "projects/travel/sunset.jpg",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual, err := NormalizeLogicalPathKey(testCase.rootPath, testCase.physicalPath)
			if err != nil {
				t.Fatalf("normalize logical path key: %v", err)
			}
			if actual != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, actual)
			}
		})
	}
}
