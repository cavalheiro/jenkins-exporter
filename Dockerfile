FROM golang:alpine

WORKDIR /go/src/jenkins-metrics
COPY . .

RUN go install -v .

VOLUME ["/go/etc"]

ENTRYPOINT [ "/go/bin/jenkins-metrics" ]
CMD [ "-config=/go/etc/config.toml" ]