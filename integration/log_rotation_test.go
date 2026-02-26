//go:build !windows

package integration

import (
	"bufio"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/hanzoai/ingress/v3/integration/try"
)

const ingressTestAccessLogFileRotated = ingressTestAccessLogFile + ".rotated"

// Log rotation integration test suite.
type LogRotationSuite struct{ BaseSuite }

func TestLogRotationSuite(t *testing.T) {
	suite.Run(t, new(LogRotationSuite))
}

func (s *LogRotationSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	os.Remove(ingressTestAccessLogFile)
	os.Remove(ingressTestLogFile)
	os.Remove(ingressTestAccessLogFileRotated)

	s.createComposeProject("access_log")
	s.composeUp()
}

func (s *LogRotationSuite) TearDownSuite() {
	s.BaseSuite.TearDownSuite()

	generatedFiles := []string{
		ingressTestLogFile,
		ingressTestAccessLogFile,
		ingressTestAccessLogFileRotated,
	}

	for _, filename := range generatedFiles {
		if err := os.Remove(filename); err != nil {
			log.Warn().Err(err).Send()
		}
	}
}

func (s *LogRotationSuite) TestAccessLogRotation() {
	// Start Traefik
	cmd, _ := s.cmdIngress(withConfigFile("fixtures/access_log/access_log_base.toml"))
	defer s.displayIngressLogFile(ingressTestLogFile)

	// Verify Traefik started ok
	s.verifyEmptyErrorLog("ingress.log")

	s.waitForTraefik("server1")

	// Make some requests
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8000/", nil)
	require.NoError(s.T(), err)
	req.Host = "frontend1.docker.local"

	err = try.Request(req, 500*time.Millisecond, try.StatusCodeIs(http.StatusOK), try.HasBody())
	require.NoError(s.T(), err)

	// Rename access log
	err = os.Rename(ingressTestAccessLogFile, ingressTestAccessLogFileRotated)
	require.NoError(s.T(), err)

	// in the midst of the requests, issue SIGUSR1 signal to server process
	err = cmd.Process.Signal(syscall.SIGUSR1)
	require.NoError(s.T(), err)

	// continue issuing requests
	err = try.Request(req, 500*time.Millisecond, try.StatusCodeIs(http.StatusOK), try.HasBody())
	require.NoError(s.T(), err)
	err = try.Request(req, 500*time.Millisecond, try.StatusCodeIs(http.StatusOK), try.HasBody())
	require.NoError(s.T(), err)

	// Verify access.log.rotated output as expected
	s.logAccessLogFile(ingressTestAccessLogFileRotated)
	lineCount := s.verifyLogLines(ingressTestAccessLogFileRotated, 0, true)
	assert.GreaterOrEqual(s.T(), lineCount, 1)

	// make sure that the access log file is at least created before we do assertions on it
	err = try.Do(1*time.Second, func() error {
		_, err := os.Stat(ingressTestAccessLogFile)
		return err
	})
	assert.NoError(s.T(), err, "access log file was not created in time")

	// Verify access.log output as expected
	s.logAccessLogFile(ingressTestAccessLogFile)
	lineCount = s.verifyLogLines(ingressTestAccessLogFile, lineCount, true)
	assert.Equal(s.T(), 3, lineCount)

	s.verifyEmptyErrorLog(ingressTestLogFile)
}

func (s *LogRotationSuite) logAccessLogFile(fileName string) {
	output, err := os.ReadFile(fileName)
	require.NoError(s.T(), err)
	log.Info().Msgf("Contents of file %s\n%s", fileName, string(output))
}

func (s *LogRotationSuite) verifyEmptyErrorLog(name string) {
	err := try.Do(5*time.Second, func() error {
		ingressLog, e2 := os.ReadFile(name)
		if e2 != nil {
			return e2
		}
		assert.Empty(s.T(), string(ingressLog))

		return nil
	})
	require.NoError(s.T(), err)
}

func (s *LogRotationSuite) verifyLogLines(fileName string, countInit int, accessLog bool) int {
	rotated, err := os.Open(fileName)
	require.NoError(s.T(), err)
	rotatedLog := bufio.NewScanner(rotated)
	count := countInit
	for rotatedLog.Scan() {
		line := rotatedLog.Text()
		if accessLog {
			if len(line) > 0 {
				if !strings.Contains(line, "/api/rawdata") {
					s.CheckAccessLogFormat(line, count)
					count++
				}
			}
		} else {
			count++
		}
	}

	return count
}
