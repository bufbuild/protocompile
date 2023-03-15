#!/usr/bin/env bash

go test -bench '^BenchmarkGoogleapisProto(?:compile|parse)(?:Canonical|NoSourceInfo)?$' -run 'match-nothing' -timeout 20m -count 1 > new-benchmark.txt

# The ci-benchmark.txt is the benchmark from github action runner (https://github.com/bufbuild/protocompile/actions/runs/4420308905/jobs/7749835161)
benchstat ci-benchmark.txt new-benchmark.txt > benchstat_result.txt
result=$(sed '1,5d;$d' benchstat_result.txt)

while IFS= read -r line; do
  diff=$(echo "$line" | awk '{print $8}')
  if [ "$diff" != '~' ] ; then
    diff="${diff:1:${#diff}-2}"
    if (( $(echo "$diff > 200" | bc -l) )) ; then
      echo "There is a benchmark that slows the operation by more than 2%."
      exit 1
    fi
  fi
done <<< "$result"