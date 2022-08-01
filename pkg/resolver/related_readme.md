**Search Term** = Resource(s) searched for from the search bar. 
Ex: "`kind:Deployment, name:search-api` refers to the search-api deployment.

Related function finds resources related to the search term (search-api pod in the above example)


```         
                    MANAGED CLUSTER                    |    HUB CLUSTER
                                                       |
Service-->Pod-->Replicaset-->Deployment-->Subscription-|->Subscription<--Application
   |        |                    ^                   ^ |
   |        |____________________|___________________| |
   |_____________________________|___________________| |
                                                       |
```
Comparing the database structure to a tree, Search, can surface relationships on either side of the search term. The search depth can be controlled by the user by setting the `RELATION_LEVEL` environment variable. If `RELATION_LEVEL` is not set, there are 2 paths of execution. 

By default, Search will surface relationships 1 level deep on either side of the search term.
Searching for the `Pod` in the managed cluster above will bring back the Service, Replicaset, Deployment and Subscription.

**NORMAL EXECUTION - Depth 1**

Query:
```
SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search"."edges" AS "e" 
	   WHERE (("destid" IN (<UID(s)>)) 
			  OR ("sourceid" IN (<UID(s)>)))
```

`edges` view stores all these relationships between the resources. Querying the view surfaces the related resources for the Search term (identified by the UID(s)).

**FOR APPLICATIONS - Depth 3**

If the search term involves Applications within input filters or relatedKinds, Search will surface relationships 3 levels deep on either side of the search term.
Searching for the `Application` in the hub cluster above will bring back the Service, Replicaset, Deployment and both Subscriptions. This is done by recursively querying the  `edges` view.

```
WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS
------------------------NON-RECURSIVE PART START------------------------
		(SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" 
		 FROM "search"."edges" AS "e" 
		 WHERE (("destid" IN (<UID(s)>)) OR 
					("sourceid" IN (<UID(s)>)) --Input /Control condition for non-recursive part
			   ) 
------------------------NON-RECURSIVE PART END------------------------
				   UNION 
-------------------------RECURSIVE PART START------------------------
		 (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster" 
		  FROM "search"."edges" AS "e" 
		  INNER JOIN "search_graph" AS "sg" 
		  ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR 
			  ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))
			 ) 
 		  WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND  -- Avoid Node and Channel to prevent all resources on the node from showing up
				 ("sg"."level" <= 3) --Limit level to 3
 				)
------------------------RECURSIVE PART START------------------------
		 )
		) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph"
```



