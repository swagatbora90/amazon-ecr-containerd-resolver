#!/bin/bash -e
set -xe

declare -a pull_times
declare -a speeds
declare -a memories

PATH=/usr/local/go/bin:/home/ec2-user/.local/bin:/home/ec2-user/bin:/usr/local/bin:/usr/bin:/usr/local/sbin:/usr/sbin:/usr/local/go/bin
sudo rm -rf results.json
sudo rm -rf results_averages.json
sudo rm -rf ./bin
sudo PATH=/usr/local/go/bin:$PATH make build
img="ecr.aws/arn:aws:ecr:us-west-2:167340705217:repository/3gb-single-layer:latest"
img1="167340705217.dkr.ecr.us-west-2.amazonaws.com/3gb-single-layer:latest"
echo $img >> results_averages.json
#for i in 0 1 2 3 4 5 10 20 30 40 50 100; do
for i in $(seq 1 1); do
set -e
echo "{" >> results.json
for j in $(seq 1 1); do    
  >&2 echo "Run: $j with parallel arg: $i"
  ECR_PULL_PARALLEL=$i
  CTD_PARALLELISM=$i
  CTD_CHUNKSIZE_MB=20
  >&2 sudo service containerd stop
  >&2 sudo rm -rf /var/lib/containerd
  >&2 sudo mkdir -p /var/lib/containerd
  >&2 sudo service containerd start
  sudo rm -rf cpu.prof
  sudo ctr pprof profile --seconds 70s  > outputNew.prof &
  CGROUP_PARENT="ecr-pull-benchmark"
  CGROUP_CHILD="count-$j-parallel-${ECR_PULL_PARALLEL}-slice"
  #CGROUP_PARENT="docker-pull-benchmark"
  #CGROUP_CHILD="count-$j-parallel-${CTD_PARALLELISM}-slice"
  CGROUP=${CGROUP_PARENT}/${CGROUP_CHILD}
  IMAGE_URL=$img
  sudo mkdir -p /sys/fs/cgroup/${CGROUP}
  sudo echo '+memory' | sudo tee /sys/fs/cgroup/${CGROUP_PARENT}/cgroup.subtree_control
  sudo echo '+cpu' | sudo tee  /sys/fs/cgroup/${CGROUP_PARENT}/cgroup.subtree_control
  OUTPUT_FILE="/tmp/${CGROUP_CHILD}"
  sudo ./test.sh ${CGROUP} ${OUTPUT_FILE} sudo ECR_PULL_PARALLEL="${ECR_PULL_PARALLEL}" ./bin/ecr-pull ${IMAGE_URL}
  #sudo ./test.sh ${CGROUP} ${OUTPUT_FILE} sudo CTD_PARALLELISM="${CTD_PARALLELISM}" CTD_CHUNKSIZE_MB="${CTD_CHUNKSIZE_MB}" ./bin/ecr-pull ${IMAGE_URL}
  ELAPSED=$(grep elapsed ${OUTPUT_FILE}| tail -n 1)
  NETWORK_TIME=$(grep 'Network Download' ${OUTPUT_FILE} | tail -n 1 | awk -F ' ' '{print $NF}')
  # PLSM=$(grep getdownloadURL ${OUTPUT_FILE} | tail -n 1)
  TIME=$(cut -d" " -f 2 <<< "${ELAPSED}" | sed -e 's/s//')
  SPEED=$(sed -e 's/.*(//' -e 's/)//' <<< "${ELAPSED}" | cut -d" " -f 1)
  >&2 echo "${ELAPSED}"
  MEMORY=$(cat /sys/fs/cgroup/${CGROUP}/memory.peak)
  CPU=$(cat /sys/fs/cgroup/${CGROUP}/cpu.stat)
  echo "Parallel: ${ECR_PULL_PARALLEL},Time: ${TIME},Network time: ${NETWORK_TIME}, Speed: ${SPEED},Memory: ${MEMORY}, PLSM: ${PLSM}"
  #echo "CTD_PARALLELISM: ${CTD_PARALLELISM}, CTD_CHUNKSIZE_MB: ${CTD_CHUNKSIZE_MB},Time: ${TIME}, Network time: ${NETWORK_TIME}, Speed: ${SPEED},Memory: ${MEMORY}"
  tot_Mem=$(( ${MEMORY} / 1048576 ))
  echo "\"run-$i\" : {
    \"Pull Time\": ${TIME},
    \"Network Pull time\": ${NETWORK_TIME},
    \"Speed\": ${SPEED},
    \"Memory\": ${tot_Mem},
    \"CTD_PARALLELISM\":  ${CTD_PARALLELISM},
  }," >> results.json
  echo "tot_Mem = ${tot_Mem}"
  if [ ${tot_Mem} -ne 0 ]
  then
   pull_times+=("$TIME")
   speeds+=("$SPEED")
   memories+=("$tot_Mem")
  fi
  #sudo rm ${OUTPUT_FILE}
  sudo rmdir /sys/fs/cgroup/${CGROUP}
done
 echo "}" >> results.json
 pull_time_avg=$(echo "${pull_times[@]}" | tr ' ' '\n' | awk '{sum+=$1} END {print sum/NR}')
 speed_avg=$(echo "${speeds[@]}" | tr ' ' '\n' | awk '{sum+=$1} END {print sum/NR}')
 memory_avg=$(echo "${memories[@]}" | tr ' ' '\n' | awk '{sum+=$1} END {print sum/NR}')
 total_pull_time=$(echo "${pull_times[@]}" | tr ' ' '\n' | awk '{sum+=$1} END {print sum}')
 total_speed=$(echo "${speeds[@]}" | tr ' ' '\n' | awk '{sum+=$1} END {print sum}')
 total_memory=$(echo "${memories[@]}" | tr ' ' '\n' | awk '{sum+=$1} END {print sum}')
 echo "\"Averages\":  {
    \"Parallel layers\": $(( 7 - $i)),
    \"Pull Time\": ${pull_time_avg},
    \"Speed\": ${speed_avg},
    \"Memory\": ${memory_avg},
    \"Total_Download_time\": ${total_pull_time},
    \"Total_speed\": ${total_speed},
    \"Total_mem\": ${total_memory}
 }," >> results_averages.json

 pull_times=()
 speeds=()
 memories=()
done

