package aws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

// Grab any existing AWS keys and preserve. In some tests we'll unset these, so
// we need to have them and restore them after
var k = os.Getenv("AWS_ACCESS_KEY_ID")
var s = os.Getenv("AWS_SECRET_ACCESS_KEY")
var to = os.Getenv("AWS_SESSION_TOKEN")

func TestAWSConfig_shouldError(t *testing.T) {
	unsetEnv(t)
	defer resetEnv(t)
	cfg := Config{}

	c := getCreds(cfg.AccessKey, cfg.SecretKey, cfg.Token)
	_, err := c.Get()
	if awsErr, ok := err.(awserr.Error); ok {
		if awsErr.Code() != "NoCredentialProviders" {
			t.Fatalf("Expected NoCredentialProviders error")
		}
	}
	if err == nil {
		t.Fatalf("Expected an error with empty env, keys, and IAM in AWS Config")
	}
}

func TestAWSConfig_shouldBeStatic(t *testing.T) {
	simple := []struct {
		Key, Secret, Token string
	}{
		{
			Key:    "test",
			Secret: "secret",
		}, {
			Key:    "test",
			Secret: "test",
			Token:  "test",
		},
	}

	for _, c := range simple {
		cfg := Config{
			AccessKey: c.Key,
			SecretKey: c.Secret,
			Token:     c.Token,
		}

		creds := getCreds(cfg.AccessKey, cfg.SecretKey, cfg.Token)
		if creds == nil {
			t.Fatalf("Expected a static creds provider to be returned")
		}
		v, err := creds.Get()
		if err != nil {
			t.Fatalf("Error gettings creds: %s", err)
		}
		if v.AccessKeyID != c.Key {
			t.Fatalf("AccessKeyID mismatch, expected: (%s), got (%s)", c.Key, v.AccessKeyID)
		}
		if v.SecretAccessKey != c.Secret {
			t.Fatalf("SecretAccessKey mismatch, expected: (%s), got (%s)", c.Secret, v.SecretAccessKey)
		}
		if v.SessionToken != c.Token {
			t.Fatalf("SessionToken mismatch, expected: (%s), got (%s)", c.Token, v.SessionToken)
		}
	}
}

// TestAWSConfig_shouldIAM is designed to test the scenario of running Terraform
// from an EC2 instance, without environment variables or manually supplied
// credentials.
func TestAWSConfig_shouldIAM(t *testing.T) {
	// clear AWS_* environment variables
	unsetEnv(t)
	defer resetEnv(t)

	// capture the test server's close method, to call after the test returns
	ts := awsEnv(t)
	defer ts()

	// An empty config, no key supplied
	cfg := Config{}

	creds := getCreds(cfg.AccessKey, cfg.SecretKey, cfg.Token)
	if creds == nil {
		t.Fatalf("Expected a static creds provider to be returned")
	}

	v, err := creds.Get()
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	if v.AccessKeyID != "somekey" {
		t.Fatalf("AccessKeyID mismatch, expected: (somekey), got (%s)", v.AccessKeyID)
	}
	if v.SecretAccessKey != "somesecret" {
		t.Fatalf("SecretAccessKey mismatch, expected: (somesecret), got (%s)", v.SecretAccessKey)
	}
	if v.SessionToken != "sometoken" {
		t.Fatalf("SessionToken mismatch, expected: (sometoken), got (%s)", v.SessionToken)
	}
}

// TestAWSConfig_shouldIAM is designed to test the scenario of running Terraform
// from an EC2 instance, without environment variables or manually supplied
// credentials.
func TestAWSConfig_shouldIgnoreIAM(t *testing.T) {
	unsetEnv(t)
	defer resetEnv(t)
	// capture the test server's close method, to call after the test returns
	ts := awsEnv(t)
	defer ts()
	simple := []struct {
		Key, Secret, Token string
	}{
		{
			Key:    "test",
			Secret: "secret",
		}, {
			Key:    "test",
			Secret: "test",
			Token:  "test",
		},
	}

	for _, c := range simple {
		cfg := Config{
			AccessKey: c.Key,
			SecretKey: c.Secret,
			Token:     c.Token,
		}

		creds := getCreds(cfg.AccessKey, cfg.SecretKey, cfg.Token)
		if creds == nil {
			t.Fatalf("Expected a static creds provider to be returned")
		}
		v, err := creds.Get()
		if err != nil {
			t.Fatalf("Error gettings creds: %s", err)
		}
		if v.AccessKeyID != c.Key {
			t.Fatalf("AccessKeyID mismatch, expected: (%s), got (%s)", c.Key, v.AccessKeyID)
		}
		if v.SecretAccessKey != c.Secret {
			t.Fatalf("SecretAccessKey mismatch, expected: (%s), got (%s)", c.Secret, v.SecretAccessKey)
		}
		if v.SessionToken != c.Token {
			t.Fatalf("SessionToken mismatch, expected: (%s), got (%s)", c.Token, v.SessionToken)
		}
	}
}

func TestAWSConfig_shouldBeENV(t *testing.T) {
	// need to set the environment variables to a dummy string, as we don't know
	// what they may be at runtime without hardcoding here
	s := "some_env"
	setEnv(s, t)
	defer resetEnv(t)

	cfg := Config{}
	creds := getCreds(cfg.AccessKey, cfg.SecretKey, cfg.Token)
	if creds == nil {
		t.Fatalf("Expected a static creds provider to be returned")
	}
	v, err := creds.Get()
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	if v.AccessKeyID != s {
		t.Fatalf("AccessKeyID mismatch, expected: (%s), got (%s)", s, v.AccessKeyID)
	}
	if v.SecretAccessKey != s {
		t.Fatalf("SecretAccessKey mismatch, expected: (%s), got (%s)", s, v.SecretAccessKey)
	}
	if v.SessionToken != s {
		t.Fatalf("SessionToken mismatch, expected: (%s), got (%s)", s, v.SessionToken)
	}
}

// unsetEnv unsets enviornment variables for testing a "clean slate" with no
// credentials in the environment
func unsetEnv(t *testing.T) {
	if err := os.Unsetenv("AWS_ACCESS_KEY_ID"); err != nil {
		t.Fatalf("Error unsetting env var AWS_ACCESS_KEY_ID: %s", err)
	}
	if err := os.Unsetenv("AWS_SECRET_ACCESS_KEY"); err != nil {
		t.Fatalf("Error unsetting env var AWS_SECRET_ACCESS_KEY: %s", err)
	}
	if err := os.Unsetenv("AWS_SESSION_TOKEN"); err != nil {
		t.Fatalf("Error unsetting env var AWS_SESSION_TOKEN: %s", err)
	}
}

func resetEnv(t *testing.T) {
	// re-set all the envs we unset above
	if err := os.Setenv("AWS_ACCESS_KEY_ID", k); err != nil {
		t.Fatalf("Error resetting env var AWS_ACCESS_KEY_ID: %s", err)
	}
	if err := os.Setenv("AWS_SECRET_ACCESS_KEY", s); err != nil {
		t.Fatalf("Error resetting env var AWS_SECRET_ACCESS_KEY: %s", err)
	}
	if err := os.Setenv("AWS_SESSION_TOKEN", to); err != nil {
		t.Fatalf("Error resetting env var AWS_SESSION_TOKEN: %s", err)
	}
}

func setEnv(s string, t *testing.T) {
	// set all the envs to a dummy value
	if err := os.Setenv("AWS_ACCESS_KEY_ID", s); err != nil {
		t.Fatalf("Error setting env var AWS_ACCESS_KEY_ID: %s", err)
	}
	if err := os.Setenv("AWS_SECRET_ACCESS_KEY", s); err != nil {
		t.Fatalf("Error setting env var AWS_SECRET_ACCESS_KEY: %s", err)
	}
	if err := os.Setenv("AWS_SESSION_TOKEN", s); err != nil {
		t.Fatalf("Error setting env var AWS_SESSION_TOKEN: %s", err)
	}
}

// awsEnv establishes a httptest server to mock out the internal AWS Metadata
// service. IAM Credentials are retrieved by the EC2RoleProvider, which makes
// API calls to this internal URL. By replacing the server with a test server,
// we can simulate an AWS environment
func awsEnv(t *testing.T) func() {
	routes := routes{}
	if err := json.Unmarshal([]byte(aws_routes), &routes); err != nil {
		t.Fatalf("Failed to unmarshal JSON in AWS ENV test: %s", err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Add("Server", "MockEC2")
		for _, e := range routes.Endpoints {
			if r.RequestURI == e.Uri {
				fmt.Fprintln(w, e.Body)
			}
		}
	}))

	os.Setenv("AWS_METADATA_URL", ts.URL)
	return ts.Close
}

type routes struct {
	Endpoints []*endpoint `json:"endpoints"`
}
type endpoint struct {
	Uri  string `json:"uri"`
	Body string `json:"body"`
}

const aws_routes = `
{
  "endpoints": [
    {
      "uri": "/meta-data/iam/security-credentials",
      "body": "test_role"
    },
    {
      "uri": "/meta-data/iam/security-credentials/test_role",
      "body": "{\"Code\":\"Success\",\"LastUpdated\":\"2015-12-11T17:17:25Z\",\"Type\":\"AWS-HMAC\",\"AccessKeyId\":\"somekey\",\"SecretAccessKey\":\"somesecret\",\"Token\":\"sometoken\"}"
    }
  ]
}
`
