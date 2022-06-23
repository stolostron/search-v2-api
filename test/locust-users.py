
# Copyright Contributors to the Open Cluster Management project
from locust import HttpUser, task, between, TaskSet
import urllib3
urllib3.disable_warnings() # Suppress warning from unverified TLS connection (verify=false)

userCount = 0
token_string = "Replace with <oc whoami -t>"

class UserBehavior(TaskSet):

    @task
    def do_keyword_search(self):
        q = {
            "query": "query q($input: [SearchInput]) {\n  searchResult: search(input: $input) {\n    items\n    __typename\n  }\n}",
            "variables": {"input":[{"keywords": ["prom"]}]}
        }

        self.client.payload = q
        self.do_post()
        print("Sent keyword search for user {}".format(self.user.name))  

    @task
    def do_filter_search(self):
        q = {
            "query": "query q($input: [SearchInput]) {\n  searchResult: search(input: $input) {\n    items\n    __typename\n  }\n}",
            "variables": {"input":[{"filters": [{"property":"kind", "values":["Pod"]}]}]}
        }
        self.client.payload = q
        self.do_post()
        print("Sent filter search for user {}".format(self.user.name))        

    @task
    def do_count_search(self):
        q = {
            "query": "query q($input: [SearchInput]) {\n  searchResult: search(input: $input) {\n    count\n    __typename\n  }\n}",
            "variables": {"input":[
                {"filters": [{"property":"kind", "values":["Pod"]}]},
                {"filters": [{"property":"kind", "values":["Deployment"]}]},
            ]}
        }
        self.client.payload = q
        self.do_post()
        print("Sent count search for user {}".format(self.user.name))

    # @task
    # def do_related_search(self):
    #     print("TODO: Send related query.")

    # @task
    # def do_autocomplete(self):
    #     print("TODO: Send autocomplete query.")

    def do_post(self):
        self.client.post("/searchapi/graphql", 
            headers={"Authorization": "Bearer " + self.user.token},
            json=self.client.payload, verify=False)

       

class User(HttpUser):
    name = ""
    token = ""
    tasks = [UserBehavior]
    wait_time = between(1,5)

    def on_start(self):
        global userCount
        self.name = "user{}".format(userCount)
        userCount = userCount + 1

        # TODO: For an accurate result, we'll need to register OCP users and obtain individual tokens.
        #       Otherwise, the results will be inacurate because of caching.
        self.token = token_string
        print("Starting user [%s]" % self.name)