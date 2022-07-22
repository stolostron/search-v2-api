#!/bin/bash
cd test
echo $API_TOKEN

locust --headless --users ${N_USERS} --spawn-rate ${N_USERS} -H https://${HOST} -f locust-users.py --loglevel DEBUG -t ${RUN_TIME} 