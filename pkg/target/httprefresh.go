package target

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/pkg/errors"
	"github.com/snowplow/snowbridge/pkg/models"
	"sync"
	"time"
)

type HTTPRefreshTarget struct {
	HTTPTarget
	headersSSMPath string
	mux            *sync.Mutex
}
type HTTPRefreshTargetConfig struct {
	HTTPTargetConfig
	headerSSMPath string `hcl:"headers_ssm_path,optional" env:"TARGET_HTTP_HEADERS_SSM_PATH" `
}

func HTTPRefreshTargetConfigFunction(c *HTTPRefreshTargetConfig) (*HTTPRefreshTarget, error) {
	t, err := newHTTPTarget(
		c.HTTPURL,
		c.RequestTimeoutInSeconds,
		c.ByteLimit,
		c.ContentType,
		c.Headers,
		c.BasicAuthUsername,
		c.BasicAuthPassword,
		c.CertFile,
		c.KeyFile,
		c.CaFile,
		c.SkipVerifyTLS,
	)

	target := &HTTPRefreshTarget{*t, c.headerSSMPath, &sync.Mutex{}}
	target.startSync()
	return target, err
}

func (h *HTTPRefreshTarget) startSync() {
	if h.headersSSMPath == "" {
		return
	}

	sess := session.Must(session.NewSession())
	client := ssm.New(sess)
	t := time.NewTicker(10 * time.Second)
	go func() {
		for ; true; <-t.C {
			resp, err := client.GetParameter(&ssm.GetParameterInput{
				Name:           aws.String(h.headersSSMPath),
				WithDecryption: aws.Bool(true),
			})
			if err != nil {
				h.log.Errorf("could refresh headers: %s", err)
				continue
			}
			if resp.Parameter.Value == nil {
				h.log.Error("value of parameter is nil")
				continue
			}
			newHeadersString := *resp.Parameter.Value
			newHeaders, err := getHeaders(newHeadersString)
			if err != nil {
				h.log.Errorf("could parse headers: %s", err)
				continue
			}
			h.mux.Lock()
			h.headers = newHeaders
			h.mux.Unlock()
		}

	}()
}

func (h *HTTPRefreshTarget) Write(messages []*models.Message) (*models.TargetWriteResult, error) {
	h.mux.Lock()
	defer h.mux.Unlock()
	return h.HTTPTarget.Write(messages)
}

func AdaptHTTPRefreshTargetFunc(f func(c *HTTPRefreshTargetConfig) (*HTTPRefreshTarget, error)) HTTPTargetAdapter {
	return func(i interface{}) (interface{}, error) {
		cfg, ok := i.(*HTTPRefreshTargetConfig)
		if !ok {
			return nil, errors.New("invalid input, expected HTTPTargetConfig")
		}

		return f(cfg)
	}
}
