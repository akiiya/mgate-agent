#!/bin/sh
# 仅用于 runner 测试：模拟 mgate.sh 的 stdout、stderr、超时、失败和大输出。
set -eu

mode="${MGATE_FAKE_MODE:-}"

case "$mode" in
  stderr)
    echo "fake stderr" >&2
    echo "fake stdout"
    ;;
  sleep)
    sleep 2
    echo "late stdout"
    ;;
  fail)
    echo "fake failure" >&2
    exit 7
    ;;
  bigout)
    i=0
    while [ "$i" -lt 8192 ]; do
      printf O
      i=$((i + 1))
    done
    i=0
    while [ "$i" -lt 8192 ]; do
      printf E >&2
      i=$((i + 1))
    done
    ;;
  secretout)
    echo "safe stdout"
    echo "token=abc"
    echo "password=123" >&2
    ;;
  *)
    printf 'args='
    first=1
    for arg in "$@"; do
      if [ "$first" -eq 0 ]; then
        printf ','
      fi
      printf '%s' "$arg"
      first=0
    done
    printf '\n'
    ;;
esac
