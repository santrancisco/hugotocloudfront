__Example__ : [https://blog.ebfe.pw/](https://blog.ebfe.pw/)

### HUGOTOS3

This serverless application is designed to catch webhook push event from github and subsequently build the site using [Hugo](https://gohugo.io/). The generated site is then copied to an S3 bucket, ready to be served by AWS Cloudfront

There are a few TODO using Terraform for `infrastructure as code` completeness :

  - Cloudfront CDN ✅
  - S3 bucket ✅
  - AWS Certificate Management - Acquire free cert using DNS validation (This is done manually at the moment.)
  - Route53 (I'm managing this seperately with my other domains ✅)


NOTE: 
 - At the time of writing, a hugo dependency introduced some breaking change. We can revert this by going to $GOPATH/src/github.com/spf13/jwalterweatherman and `git clone 4a4406e478ca629068e7768fc33f3f044173c0a6` to temporary fix it. 
 - Choose a webhook secret before deploy then afterward, make sure the same secret is used for the github webhook. Also make sure that the webhook is sent with `application/json` format.
 - Obtains the cloudfront url from distribution-settings and use that for our CNAME record.