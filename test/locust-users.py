
# Copyright Contributors to the Open Cluster Management project
from locust import HttpUser, task, between, TaskSet
import json
import urllib3
urllib3.disable_warnings() # Suppress warning from unverified TLS connection (verify=false)

userCount = 0
token_string = "Replace with <oc whoami -t>"

class UserBehavior(TaskSet):

    @task
    def do_search(self):
        f = open("search-query-template.json",)
        j = json.load(f)
        self.client.payload = j
        self.client.headers
        self.do_post()
        print("Sent search for user {}".format(self.user.name))        

    def do_post(self):
        self.client.post("/searchapi/graphql", 
            headers={"Authorization": "Bearer " + token_string},
            json=self.client.payload, verify=False)

       

class User(HttpUser):
    name = ""
    token_string=""
    tasks = [UserBehavior]
    wait_time = between(1,10)

    def on_start(self):
        global userCount
        self.name = "user{}".format(userCount)
        userCount = userCount + 1

        # TODO: For an accurate result, we'll need to register OCP users and obtain individual tokens.
        #       Otherwise, the results will be inacurate because of caching.
        self.token_string = token_string
        print("Starting user [%s]" % self.name)