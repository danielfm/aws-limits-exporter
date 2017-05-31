# K8S AWS Limits Monitor

This is a small Go server that provides a Prometheus metrics endpoint that
exposes AWS usage and limits as reported by the AWS Trusted Advisor API.

### AWS Credentials

For the controller to work, it must have access to AWS credentials in
`~/.aws/credentials`, or via `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`
environment variables.

The following IAM policy describes which actions the user must be able to
perform in order for the controller to work:

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
Usage of ./bin/kube-aws-limits-monitor:
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

## License

Copyright (C) Daniel Fernandes Martins

Distributed under the New BSD License. See LICENSE for further details.
