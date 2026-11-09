package strippedbackfill

// ContainerDFSConfig is the dfs yaml the production compose image actually
// runs. teamgramd/bin/runall-docker.sh starts dfs with ../etc2/dfs.yaml,
// and the process working directory is /app/bin, so the file is
// /app/etc2/dfs.yaml. /app/etc/dfs.yaml is the host-network file.
const ContainerDFSConfig = "/app/etc2/dfs.yaml"

// ContainerRedis is the cache address in teamgramd/etc2/dfs.yaml. On the
// compose network the service name is redis. 127.0.0.1:6379 is the
// teamgram container itself, not Redis.
const ContainerRedis = "redis:6379"
