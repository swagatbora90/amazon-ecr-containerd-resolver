#!/bin/bash -e

# Constants
IMG_NAME="3gb-single-layer-new"
IMG="ecr.aws/arn:aws:ecr:us-west-2:167340705217:repository/${IMG_NAME}:latest"

ECR_IMG="167340705217.dkr.ecr.us-west-2.amazonaws.com/3gb-single-layer:latest"

# Clean up previous results and build the project
sudo mkdir -p $IMG_NAME
sudo chmod -R 777 $IMG_NAME
sudo rm -if $IMG_NAME/results.csv
sudo rm -if $IMG_NAME/results-cgroup.csv
sudo rm -rf ./bin
sudo PATH=/usr/local/go/bin:$PATH make build

# Prepare CSV file
echo "Run,ParallelLayers,DownloadTime,Unpack,Speed" > $IMG_NAME/results.csv

# Main loop
#for i in 100 50 20 10 5 4 3 2 1 0; do
for i in 0; do
  for j in $(seq 1 15); do
    echo "Run: $j with parallel arg: $i" >&2

    # Set up environment
    ECR_PULL_PARALLEL=$i
    >&2 sudo service containerd stop
    >&2 sudo rm -rf /var/lib/containerd
    >&2 sudo mkdir -p /var/lib/containerd
    >&2 sudo service containerd start
    CGROUP_PARENT="ecr-pull-benchmark"
    CGROUP_CHILD="count-$j-parallel-${ECR_PULL_PARALLEL}-slice"
    CGROUP=${CGROUP_PARENT}/${CGROUP_CHILD}
    sudo mkdir -p /sys/fs/cgroup/${CGROUP}
    sudo echo '+memory' | sudo tee /sys/fs/cgroup/${CGROUP_PARENT}/cgroup.subtree_control
    sudo echo '+cpu' | sudo tee  /sys/fs/cgroup/${CGROUP_PARENT}/cgroup.subtree_control
    OUTPUT_FILE="/tmp/${CGROUP_CHILD}"

    # Pull image and collect data
    sudo ./test.sh ${CGROUP} ${OUTPUT_FILE} sudo ECR_PULL_PARALLEL="${ECR_PULL_PARALLEL}" ./bin/ecr-pull ${ECR_IMG}
    ELAPSED=$(grep elapsed ${OUTPUT_FILE} | tail -n 1)
    UNPACK=$(grep unpackTime ${OUTPUT_FILE}| tail -n 1)
    TIME=$(cut -d" " -f 2 <<< "${ELAPSED}" | sed -e 's/s//')
    UNPACKTIME=$(cut -d" " -f 2 <<< "${UNPACK}" | sed -e 's/s//')
    NETWORK_TIME=$(grep 'Network Download' ${OUTPUT_FILE} | tail -n 1 | awk -F ' ' '{print $NF}')
    SPEED=$(sed -e 's/.*(//' -e 's/)//' <<< "${ELAPSED}" | cut -d" " -f 1)
    MEMORY=$(cat /sys/fs/cgroup/${CGROUP}/memory.peak)
    CPU=$(cat /sys/fs/cgroup/${CGROUP}/cpu.stat)
    >&2 echo "${ELAPSED}"
    if (($j > 2)); then
      # Output data
      echo "$j,$ECR_PULL_PARALLEL,$TIME,$UNPACKTIME,$SPEED" >> $IMG_NAME/results.csv
      printf "$j,Parallel: $ECR_PULL_PARALLEL : \n Memory.peak in bytes = $MEMORY \n Cpu.stats = \n $CPU" >> $IMG_NAME/results-cgroup.txt
    fi

    # Clean up
    sudo rm -f ${OUTPUT_FILE}
    sudo rmdir /sys/fs/cgroup/${CGROUP}
  done
done

calculate_percentiles() {
  local values=("$@")
  local count=${#values[@]}

  p0="${values[0]}"
  p50="${values[$((count / 2))]}"

  local index_p90=$((count * 90 / 100))
  if (( index_p90 >= count )); then
    p90="${values[$((count - 1))]}"
  else
    p90="${values[$index_p90]}"
  fi

  p100="${values[$((count - 1))]}"

  echo "$p0,$p50,$p90,$p100"
}

extract_and_calculate() {
  local column=$1
  local parallelism_level=$2
  local values=($(awk -F, -v col="$column" -v par="$parallelism_level" 'NR > 1 && $2 == par {print $col}' "$input_file" | sort -nr))


  calculate_percentiles "${values[@]}"
}

input_file=$IMG_NAME/results.csv
output_file=$IMG_NAME/percentiles.csv

echo "ParallelLayers,Metric,p0,p50,p90,p100" > "$output_file"

parallelism_levels=(100 50 20 10 5 4 3 2 1 0)

for par in "${parallelism_levels[@]}"; do

  pultime_percentiles=$(extract_and_calculate 3 "$par")
  echo "$par,DownloadTime,$pultime_percentiles" | tr ' ' ',' >> "$output_file"

  unpack_percentiles=$(extract_and_calculate 4 "$par")
  echo "$par,Unpack,$unpack_percentiles" | tr ' ' ',' >> "$output_file"

  speed_percentiles=$(extract_and_calculate 5 "$par")
  echo "$par,Speed,$speed_percentiles" | tr ' ' ',' >> "$output_file"
done

echo "Percentiles calculated and saved to $output_file"
