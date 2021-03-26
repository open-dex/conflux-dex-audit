#!/usr/bin/env bash
set -e

export PROJECT_DIR=$(cd `dirname "${BASH_SOURCE[0]}"`/.. && pwd)
envFile="$PROJECT_DIR/script/env.sh"
echo "Env file $envFile"

props=('cfx' 'dbaddr' 'dbpass' 'access_token' 'dexadmin' 'dexstart' 'AesSecret')

function createEnv() {
  echo '#!/usr/bin/env bash'  >> ${envFile}
  for (( i = 0; i < ${#props[@]}; ++i )); do
    echo "export "${props[i]}"=" >> ${envFile}
  done
  echo ''  >> ${envFile}
  echo "create env file at $envFile"
}
function checkAllSet(){
    ok=true
    for (( i = 0; i < ${#props[@]}; ++i )); do
        value=$(eval echo '$'${props[i]})
#        echo "${props[i]} is ${value}"
        if [[ -z $value ]]; then
            echo "Environment is empty: ${props[i]}, pleas fix it.";
            ok=false
        fi
    done
    if [[ "$ok" = true ]]; then
        echo "Load env file ok"
    else
        exit 1
    fi
}

TIMESTAMP(){
    echo $(date "+%Y-%m-%d %H:%M:%S")
}

function startMatch(){
  logTag=$(date "+%Y%m%d_%H%M%S")
  nohup $PROJECT_DIR/conflux-dex-audit matchflow trade \
#     --init true
     --cfx $cfx  \
     --matchflow http://localhost:8080 \
     --dbaddr $dbaddr \
     --dbpass $dbpass  \
     --access-token $access_token\
     --confirm-epochs 1500 \
     --epoch -1000 \
     --dexadmin $dexadmin \
     --dbuser root \
     --dexstart $dexstart \
     --AesSecret $AesSecret \
     > audit_matchflow_$logTag.log 2>&1 &
}
function startBoom(){
  logTag=$(date "+%Y%m%d_%H%M%S")
  nohup $PROJECT_DIR/conflux-dex-audit boomflow \
    --cfx $cfx \
    --matchflow http://localhost:8080 \
    --log-level debug \
    --access-token $access_token \
    > audit_boomflow_$logTag.log 2>&1 &
}
function check() {
    which="$1"
    auditCnt=`ps aux|grep "conflux-dex-audit"|grep " $which"|wc -l|sed -e 's/^[[:space:]]*//'`

    success=true
    if [[ "$auditCnt" = "0" ]]; then
        success=false
    fi
    echo "$(TIMESTAMP) do checking $which audit alive, \
 $which audit process count is $auditCnt, \
 success is $success"

    if [[ "$success" = "true" ]]; then
        return 0
    fi

    echo "try starting $which"
    if [[ "$which" == "boomflow" ]]; then
      startBoom
    else
      startMatch
    fi
    if [[ "$2" == "loop" ]]; then
      echo "sleep after do restart $which"
      sleep 10
    fi
}
if [[ -f $envFile ]]; then
    source $envFile
    checkAllSet
else
    echo "Env file not found, now create."
    createEnv
    echo "Please config env in $envFile"
    exit 1
fi

check boomflow
check matchflow
