__Example__ : [https://blog.ebfe.pw/](https://blog.ebfe.pw/)

### HUGOTOS3

This serverless application is designed to catch webhook push event from github and subsequently build the site using [Hugo](https://gohugo.io/). The generated site is then copied to an S3 bucket, ready to be served by AWS Cloudfront

There are a few TODO using Terraform for `infrastructure as code` completeness :

  - Cloudfront CDN
  - AWS Certificate Management
  - Route53


