FROM golang:1.20-bookworm AS builder
RUN apt-get update && apt-get install -y liblpsolve55-dev

ENV CGO_CFLAGS="-I/usr/include/lpsolve"
ENV CGO_LDFLAGS="-llpsolve55 -lm -ldl -lcolamd"
ENV GONOPROXY="github.com/btoews/*"
ENV GONOSUMDB="github.com/btoews/*"

WORKDIR /go/src/github.com/btoews/best-regions
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
	--mount=type=cache,target=/go/pkg \
    go mod download

COPY *.go ./
COPY ./graph ./graph
COPY ./cmd/best-regions ./cmd/best-regions
COPY README.md README.md
COPY script.sh script.sh
RUN --mount=type=cache,target=/root/.cache/go-build \
	--mount=type=cache,target=/go/pkg \
    go build -ldflags "-X 'main.ReadMeB64=$(cat README.md | base64)' -X 'main.ScriptB64=$(cat script.sh | base64)'" -buildvcs=false -o /usr/local/bin/best-regions ./cmd/best-regions

FROM debian:bookworm AS runner
RUN apt-get update && apt-get install -y liblpsolve55-dev
COPY --from=builder /usr/local/bin/best-regions /usr/local/bin/best-regions
CMD ["best-regions"]
