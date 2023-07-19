This is a tool for helping you decide the best fly.io regions to deploy
your apps to in order to minimize latency for users.

You can read through the following script to see how it works. Alternatively,
you can run the script directly with:

  `curl https://best-regions.fly.dev | bash`

The best-regions app is deployed to every fly.io region. Each instances
periodically pings every other instance to determine latency between regions.
You can see the results from a single region (e.g. iad) by running

  `curl -H "Fly-Prefer-Region: iad" https://best-regions.fly.dev/latency.json`

You can see latency data for all regions by running

  `curl https://best-regions.fly.dev/latencies.json`

To determine which regions are best for _your_ app, we need to know where
your users are. The script bellow queries fly.io's hosted Prometheus server
to figure out how many requests your app receives from each region. From
there, we need to figure out the optimal choise of regions.

Let's say 40% of your users are in LAX, 30% are in IAD, and 30% are in DFW.
Latencies between regions look like this:

  - IAD<->LAX = 60ms
  - IAD<->DFW = 30ms
  - LAX<->DFW = 30ms

If you can only deploy to one region, you might go with LAX since that's the
most popular region. This would give users an average latency of 27ms
(0ms x 40% + 60ms x30% + 30ms x30%). Being in the middle, DFW is actually a
better choice though, giving an average latency of 21ms
(30ms x40% + 30ms x 30% + 0ms x 30%).

It's reasonably easy to figure this out when deploying to one or two regions,
but what if your users are distributed all over the world and you want to
pick the best 10 regions to deploy to? You need to evaluate the average
latency for each possible combination of 10 out of 35 regions (35 choose
10). The best-regions app does this math for you.