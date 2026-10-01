package raftsvc_test

import "github.com/jeremyhahn/dragonboat/v4/logger"

func init() {
	for _, pkg := range []string{"config", "dragonboat", "logdb", "raft", "rsm", "transport"} {
		logger.GetLogger(pkg).SetLevel(logger.ERROR)
	}
}
