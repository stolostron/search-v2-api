
# Copyright Contributors to the Open Cluster Management project
from locust import HttpUser, task, between, TaskSet
import urllib3
import os
urllib3.disable_warnings() # Suppress warning from unverified TLS connection (verify=false)

userCount = 0

class UserBehavior(TaskSet):
    @task
    def do_keyword_search(self):
        self.do_post({
            "op": "searchByKeyword",
            "query": "query q($input: [SearchInput]) {\n  searchResult: search(input: $input) {\n    items\n    __typename\n  }\n}",
            "variables": {"input":[{"keywords": ["apiserver"], "limit": 1000}]}
        })


    @task
    def do_filter_search(self):
        self.do_post({
            "op": "searchByFilter",
            "query": "query q($input: [SearchInput]) {\n  searchResult: search(input: $input) {\n    items\n    __typename\n  }\n}",
            "variables": {"input":[{"filters": [{"property":"namespace", "values":["default"]}], "limit": 1000}]}
        })

    @task
    def do_count_search(self):
        self.do_post({
            "op": "searchByCount",
            "query": "query q($input: [SearchInput]) {\n  searchResult: search(input: $input) {\n    count\n    __typename\n  }\n}",
            "variables": {"input":[
                {"filters": [{"property":"kind", "values":["Pod"]}]},
                {"filters": [{"property":"kind", "values":["Deployment"]}]},
            ]}
        })


    @task
    def do_autocomplete(self):
        self.do_post({
            "op": "searchComplete",
            "query": "query q($property: String!, $query: SearchInput, $limit: Int) {\n  searchComplete(property: $property, query: $query, limit: $limit)\n}",
            "variables": {"property": "name", "limit": 1000},
        })

    @task
    def do_search_related_count(self):
        self.do_post({
            "op": "searchRelatedCount",
            "query": "query q($input: [SearchInput]) {\n  searchResult: search(input: $input) {\n    related {\n      kind\n      count\n      __typename\n    }\n    __typename\n  }\n}",
            "variables": {"input":[
                {"filters": [{"property": "name","values": ["apiserver"]}], "limit": 1000},
            ]}
        })

    @task
    def do_search_related_items(self):
        self.do_post({
            "op": "searchRelatedItems",
            "query": "query q($input: [SearchInput]) {\n  searchResult: search(input: $input) {\n    related {\n      kind\n      items\n      __typename\n    }\n    __typename\n  }\n}",
            "variables": {"input":[
                {"filters": [{"property": "kind","values": ["Pod"]}], "limit": 1000},
            ]}
        })


    def do_post(self, payload):
        # Note: the query parameter op is a hack to report graphql queries separately. This is ignored by the search api.
        response = self.client.post("/searchapi/graphql?op={}".format(payload["op"]), 
            headers={"Authorization": "Bearer " + self.user.token},
            json=payload, verify=False)
        # Enable the following line to debug the requests from the terminal.
        # print("Sent {} for user {}".format(payload["op"], self.user.name))
        
        # Enable the following line if you want to validate the response content.
        # print("Response content: {}".format(response.content)) 
    

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
        self.token = os.getenv('API_TOKEN')
        if not self.token:
            print("\n\nCONFIGURATION ERROR!")
            print("The environment variable API_TOKEN must be set with the value of <oc whoami -t>\n")
            os._exit(1)
        print("Starting user [%s]" % self.name)