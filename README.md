# AWS Limits Exporter

This is a small Go server that provides a Prometheus metrics endpoint that
exposes AWS usage and limits as reported by the AWS Trusted Advisor API.

Checks are automatically refreshed according to the `MillisUntilNextRefreshable`
field of [TrustedAdvisorCheckRefreshStatus](http://docs.aws.amazon.com/sdk-for-go/api/service/support/#TrustedAdvisorCheckRefreshStatus).

## Usage

There are Docker images ready for use:

```bash
# Start exporter container
$ docker run -it --rm -p 8080:8080 -e AWS_ACCESS_KEY=<KEY> -e AWS_SECRET_ACCESS_KEY=<SECRET> \
      danielfm/aws-limits-exporter:latest -logtostderr -region=<REGION>

# Scrape metrics
$ curl http://localhost:8080/metrics
```

## AWS Credentials

For this to work, it must have access to AWS credentials in
`~/.aws/credentials`, or via `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`
environment variables.

The following IAM policy describes which actions the user must be able to
perform in order for this server to work:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "support:*"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}
```

## Flags

```
Usage of ./bin/aws-limits-exporter:
  -alsologtostderr
        log to standard error as well as files
  -listen-address string
        The address to listen on for HTTP requests. (default ":8080")
  -log_backtrace_at value
        when logging hits line file:N, emit a stack trace
  -log_dir string
        If non-empty, write log files in this directory
  -logtostderr
        log to standard error instead of files
  -region string
        Returns usage and limits for the given AWS Region. (default "us-east-1")
  -stderrthreshold value
        logs at or above this threshold go to stderr
  -v value
        log level for V logs
  -vmodule value
        comma-separated list of pattern=N settings for file-filtered logging
```

## Donate

If this project is useful for you, buy me a beer!

Bitcoin: `bc1qtwyfcj7pssk0krn5wyfaca47caar6nk9yyc4mu`

## License

Copyright (C) Daniel Fernandes Martins

Distributed under the New BSD License. See LICENSE for further details.
