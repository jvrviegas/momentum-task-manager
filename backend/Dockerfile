# Build from the repository root, which holds core/:
#   docker build -f backend/Dockerfile --build-arg VERSION=$(git rev-parse --short HEAD) .
FROM golang:1.27.2 AS build
# VERSION is what GET /v1/version answers.
ARG VERSION=dev
WORKDIR /src
COPY core/ core/
COPY backend/ backend/
WORKDIR /src/backend
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-X main.Version=${VERSION}" -o /momentum-server ./cmd/momentum-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /momentum-server /momentum-server
ENTRYPOINT ["/momentum-server"]
