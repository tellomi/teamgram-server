#!/usr/bin/env bash
# Install the Maestro CLI into $HOME/.maestro. Callers still export PATH.
set -euo pipefail

if ! java -version >/dev/null 2>&1; then
  if [[ -x /usr/libexec/java_home ]]; then
    export JAVA_HOME
    JAVA_HOME="$(/usr/libexec/java_home)"
    export PATH="$JAVA_HOME/bin:$PATH"
  fi
fi
if ! java -version >/dev/null 2>&1; then
  echo "Maestro needs a JDK on PATH." >&2
  exit 1
fi

if [[ ! -x "${HOME}/.maestro/bin/maestro" ]]; then
  curl -fsSL "https://get.maestro.mobile.dev" | bash
fi

export PATH="${PATH}:${HOME}/.maestro/bin"
maestro --version
