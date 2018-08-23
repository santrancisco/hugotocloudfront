package main

import (
	"archive/zip"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/gohugoio/hugo/commands"
	"gopkg.in/go-playground/webhooks.v5/github"
)

var (
	githuburl    = "https://github.com/santrancisco/ebfe_site.git"
	reponame     = "ebfe_site"
	owner        = "santrancisco"
	githubzipurl = "https://github.com/santrancisco/ebfe_site/archive/master.zip"
	bucketname   = "www.ebfe.pw"
	temp         = "/tmp/temp"
	hugofolder   = ""
	region       = "us-east-1"
	sitehost     = "www.ebfe.pw"
	archivepath  = "/tmp/site.zip"
)

func init() {
	if os.Getenv("BUCKETNAME") != "" {
		bucketname = os.Getenv("BUCKETNAME")
	}
	if os.Getenv("REPONAME") != "" {
		reponame = os.Getenv("REPONAME")
	}
	if os.Getenv("OWNER") != "" {
		owner = os.Getenv("OWNER")
	}
	if os.Getenv("REGION") != "" {
		region = os.Getenv("REGION")
	}
	if os.Getenv("SITEHOST") != "" {
		sitehost = os.Getenv("SITEHOST")
	}
	githuburl = "https://github.com/" + owner + "/" + reponame + ".git"
	githubzipurl = "https://github.com/" + owner + "/" + reponame + "/archive/master.zip"
	hugofolder = temp + "/" + reponame + "-master"
}

func action() {
	// git binary does not exist on lambda docker container :(
	// err := downloadprojectviagit()
	// if err != nil {
	// 	os.Stderr.WriteString(err.Error())
	// 	return
	// }
	err := downloadprojectviazip()
	if err != nil {
		os.Stderr.WriteString(err.Error())
		return
	}
	log.Println("Building site using Hugo")
	err = buildsite()
	if err != nil {
		os.Stderr.WriteString(err.Error())
		return
	}
	log.Println("Uploading site to S3 bucket")
	err = updatesite()
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				os.Stderr.WriteString(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			os.Stderr.WriteString(err.Error())
		}
		os.Stderr.WriteString(err.Error())
		return
	}
}

func lambdaHandler(event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if event.HTTPMethod != "POST" {
		return events.APIGatewayProxyResponse{Body: "Not expecting this type of request", StatusCode: 400}, nil
	}
	originalrequest := getoriginalrequest(event)
	parsegithubwebhook(originalrequest)
	return events.APIGatewayProxyResponse{Body: "Completed", StatusCode: 200}, nil
}

func main() {
	// uncomment below to test run locally without webhook
	// action()
	lambda.Start(lambdaHandler)
}

func parsegithubwebhook(r *http.Request) error {
	hook, _ := github.New(github.Options.Secret(os.Getenv("WEBHOOK_SECRET")))
	payload, err := hook.Parse(r, github.PushEvent, github.ReleaseEvent)
	if err != nil {
		if err == github.ErrEventNotFound {
			// ok event wasn;t one of the ones asked to be parsed
			// ignore the rest
			return nil
		} else {
			return err
		}
	}
	switch payload.(type) {

	case github.ReleasePayload:
		//release := payload.(github.ReleasePayload)
		// We can use this to trigger publish ONLY on release event, for now we ignore it.
		return nil

	case github.PushPayload:
		log.Println("Github Push event triggered")
		//pullRequest := payload.(github.PushPayload)
		// Let's update our blog's content on S3
		action()
	}
	return nil
}

func getoriginalrequest(proxyRequest events.APIGatewayProxyRequest) *http.Request {
	var body string
	decodedBody, err := base64.StdEncoding.DecodeString(proxyRequest.Body)
	if err != nil {
		body = proxyRequest.Body
	} else {
		body = string(decodedBody)
	}
	r := httptest.NewRequest(proxyRequest.HTTPMethod, proxyRequest.Path, strings.NewReader(body))
	// Set the host to whatever your APIG base path is
	r.Host = sitehost
	// Add the APIGatewayProxyRequest headers to the *http.Request
	for key, value := range proxyRequest.Headers {
		r.Header.Add(key, value)
	}
	// Create a x-url-formencoded string from QueryStringParameters
	var queryString string
	for key, value := range proxyRequest.QueryStringParameters {
		queryString = fmt.Sprintf("%s%s=%s&", queryString, key, value)
	}
	queryString = strings.TrimSuffix(queryString, "&")

	// Ensure all paths have a trailing slash for consistency
	path := proxyRequest.Path
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	// Create a new url.URL
	rURL := url.URL{
		Scheme:     "https",
		Opaque:     "",
		User:       nil,
		Host:       sitehost,
		Path:       path,
		RawPath:    path,
		ForceQuery: false,
		RawQuery:   queryString,
		Fragment:   "",
	}

	r.URL = &rURL
	return r
}

func unzip(archive, target string) error {
	log.Println("Unziping our site contents")
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}

	for _, file := range reader.File {
		path := filepath.Join(target, file.Name)
		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.Mode())
			continue
		}

		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()

		targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		defer targetFile.Close()

		if _, err := io.Copy(targetFile, fileReader); err != nil {
			return err
		}
	}

	return nil
}

func downloadprojectviazip() error {
	cmd := exec.Command("rm", "-rf", temp)
	_ = cmd.Run()

	if err := os.MkdirAll(temp, 0755); err != nil {
		return err
	}
	r, err := http.Get(githubzipurl)
	if err != nil {
		return err
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(archivepath, body, 0644)
	if err != nil {
		return err
	}
	defer func() {
		cmd := exec.Command("rm", "-rf", archivepath)
		_ = cmd.Run()
	}()

	// Empty out hugofolder just incase
	err = unzip(archivepath, temp)
	if err != nil {
		return err
	}
	return nil
}

func downloadprojectviagit() error {
	// Empty out hugofolder just incase
	cmd := exec.Command("rm", "-rf", hugofolder)
	err := cmd.Run()
	if err != nil {
		return err
	}
	// Clone our project to hugofolder
	cmd = exec.Command("git", "clone", githuburl, hugofolder)
	if err != nil {
		return err
	}
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func isDirectory(path string) bool {
	fd, err := os.Stat(path)
	if err != nil {
		log.Println(err)
		os.Exit(2)
	}
	switch mode := fd.Mode(); {
	case mode.IsDir():
		return true
	case mode.IsRegular():
		return false
	}
	return false
}

// This S3 upload code is from https://blog.tocconsulting.fr/upload-files-and-directories-to-aws-s3-using-golang/

func uploadDirToS3(sess *session.Session, bucketname string, bucketPrefix string, dirPath string) {
	fileList := []string{}
	filepath.Walk(dirPath, func(path string, f os.FileInfo, err error) error {
		if isDirectory(path) {
			// Do nothing
			return nil
		} else {
			fileList = append(fileList, path)
			return nil
		}
	})

	for _, file := range fileList {
		uploadFileToS3(sess, bucketname, bucketPrefix, file)
	}
}

func uploadFileToS3(sess *session.Session, bucketname string, bucketPrefix string, filePath string) {
	log.Println("upload " + filePath + " to S3")
	// An s3 service
	s3Svc := s3.New(sess)
	file, err := os.Open(filePath)
	if err != nil {
		log.Println("Failed to open file", file, err)
		os.Exit(1)
	}
	defer file.Close()
	var key string
	abspath, _ := filepath.Abs(filePath)
	fmt.Println(abspath)
	fmt.Println(hugofolder + "/public")
	key = bucketPrefix + strings.SplitAfterN(abspath, hugofolder+"/public", 2)[1]

	// Upload the file to the s3 given bucket
	params := &s3.PutObjectInput{
		Bucket: aws.String(bucketname), // Required
		Key:    aws.String(key),        // Required
		Body:   file,
	}
	_, err = s3Svc.PutObject(params)
	if err != nil {
		fmt.Printf("Failed to upload data to %s/%s, %s\n",
			bucketname, key, err.Error())
		return
	}
}

func updatesite() error {
	bucketname := bucketname
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		return err
	}
	// Create S3 service client
	log.Println("Deleting all objects in s3 bucket")
	svc := s3.New(sess)
	// create a iterator to all objects for the bucket
	iter := s3manager.NewDeleteListIterator(svc, &s3.ListObjectsInput{
		Bucket: aws.String(bucketname),
	})
	// Create bach delete for all objects
	if err := s3manager.NewBatchDeleteWithClient(svc).Delete(aws.BackgroundContext(), iter); err != nil {
		return err
	}
	log.Println("Upload our site")
	uploadDirToS3(sess, bucketname, "", hugofolder+"/public")

	return nil
}

func buildsite() error {
	// Note that this does not actually shelling out, we are simply triggering the "main" function of hugo with the options we want.
	hugoarguments := []string{"-s", hugofolder}
	fmt.Println(hugofolder)
	resp := commands.Execute(hugoarguments)
	if resp.Err != nil {
		return resp.Err
	}
	return nil
}
