// Use of this source code is governed by the MIT License,
// which can be found in the LICENSE file.

package bamboo

import (
	"encoding/json"
	"fmt"
	// "reflect"
	"gopkg.in/redis.v3"
	"testing"
	"time"
)

const NS = "TEST"

// ComprareJobs compares a subset of job fields that should not change
// over the lifetime of the job.
func CompareJobs(a *Job, b *Job) bool {
	return a.Priority == b.Priority &&
		a.JobID == b.JobID &&
		a.Payload == b.Payload &&
		a.DateAdded == b.DateAdded &&
		a.ContentType == b.ContentType &&
		a.Encoding == b.Encoding
}

func removeQueues(rjq *RJQ) {
	kqueued := MakeKey(rjq.Namespace, "QUEUED")
	kworking := MakeKey(rjq.Namespace, "WORKING")
	kscheduled := MakeKey(rjq.Namespace, "SCHEDULED")
	kfailed := MakeKey(rjq.Namespace, "FAILED")
	kworkers := MakeKey(rjq.Namespace, "WORKERS")
	kworker := MakeKey(kworkers, rjq.WorkerName)
	kworker_active := MakeKey(kworker, "ACTIVE")
	alljobs := MakeKey(rjq.Namespace, "JOBS", "*")
	kmaxjobs := MakeKey(rjq.Namespace, "MAXJOBS")
	kmaxfailed := MakeKey(rjq.Namespace, "MAXFAILED")

	rjq.Client.Del(kqueued)
	rjq.Client.Del(kworking)
	rjq.Client.Del(kscheduled)
	rjq.Client.Del(kfailed)
	rjq.Client.Del(kworkers)
	rjq.Client.Del(kworker)
	rjq.Client.Del(kworker_active)
	rjq.Client.Del(kmaxjobs)
	rjq.Client.Del(kmaxfailed)

	jobs, err := rjq.Client.Keys(alljobs).Result()
	if err != nil {
		fmt.Println(err)
	}
	rjq.Client.Del(jobs...)
}

func makeConn() *redis.Client {
	conn, _ := MakeConn("localhost", 6379, "", 0)
	return conn
}

func TestMakeKey(t *testing.T) {
	key := MakeKey("a", "b", "c")
	expected := "a:b:c"
	if key != expected {
		t.Fatal("MakeKey() Returned %s Expected %s", key, expected)
	}
}

func TestTest(t *testing.T) {
	// fmt.Printf("%d\n", time.Now().UTC().Unix())
	conn := makeConn()
	rjq := MakeQueue(NS, conn)
	defer conn.Close()
	defer removeQueues(rjq)
	// fmt.Println(rjq)
	// fmt.Println("time.Now.UTC.Unix", time.Now().UTC().Unix())
	// rjq.Test()

	// _ = rjq.Client.ZAdd(string(QUEUED), redis.Z{1, "test1"})
	// val, err := rjq.Client.ZRange(string(QUEUED), 0, int64(3)).Result()
	// fmt.Println(val)
	// fmt.Println(err)
	// _ = rjq.Client.ZRem(string(QUEUED), "test1")
}

func generateTestJobs(n int) (jobs []*Job) {
	for i := 0; i < n; i++ {
		jobid := fmt.Sprintf("job%d", i)
		data := make(map[string]string)
		data["a"] = "A"
		data["b"] = "B"
		payload, _ := json.Marshal(data)
		job := &Job{
			JobID:       jobid,
			Priority:    5,
			Payload:     string(payload),
			ContentType: "application/json",
			Owner:       "",
			DateAdded:   time.Now().UTC().Unix(),
		}

		jobs = append(jobs, job)
	}
	return jobs
}

func jobExistsInZSet(rjq *RJQ, job *Job, khmap string) bool {
	_, err := rjq.Client.ZScore(khmap, job.JobID).Result()
	if err != nil {
		return false
	}
	return true
}

func jobExistsInAnyZSet(rjq *RJQ, job *Job) bool {
	for _, queue := range []string{"SCHEDULED", "WORKING", "QUEUED", "FAILED"} {
		if jobExistsInZSet(rjq, job, MakeKey(rjq.Namespace, queue)) {
			return true
		}
	}
	return false
}

func printQueues(rjq *RJQ) {
	for _, queue := range []string{"SCHEDULED", "WORKING", "QUEUED", "FAILED"} {
		jobs, _ := rjq.Client.ZRange(MakeKey(rjq.Namespace, queue), 0, -1).Result()
		fmt.Println(queue)
		fmt.Println(jobs)
	}
}

func TestPeek(t *testing.T) {
	conn := makeConn()
	rjq := MakeQueue(NS, conn)
	defer conn.Close()
	defer removeQueues(rjq)

	// Add some items
	jobs := generateTestJobs(3)
	for _, job := range jobs {
		rjq.Add(job)
	}

	// Peek all items
	jobs2, err := rjq.Peek(3, QUEUED)
	if err != nil {
		t.Fatal(err)
	}

	for i, job := range jobs2 {
		if !CompareJobs(jobs[i], jobs2[i]) {
			t.Fatal("TestPeek: jobs don't match.")
		}
		// Remove the items
		err = rjq.Cancel(job)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestAddAndCancel(t *testing.T) {
	conn := makeConn()
	rjq := MakeQueue(NS, conn)

	job := generateTestJobs(1)[0]

	kqueued := MakeKey(rjq.Namespace, "QUEUED")
	// kworking := MakeKey(rjq.Namespace, "WORKING")
	job_key := MakeKey(rjq.Namespace, "JOBS", job.JobID)

	defer conn.Close()
	defer removeQueues(rjq)

	err := rjq.Add(job)
	if err != nil {
		t.Fatal(err)
	}

	// Test to make sure the Job data is there
	_, err = rjq.Get(job.JobID)
	if err != nil {
		t.Fatal(err)
	}

	score, err := rjq.Client.ZScore(kqueued, job.JobID).Result()
	if err != nil {
		t.Fatal(err)
	}
	if score != job.Priority {
		t.Fatal(fmt.Sprintf("Priority not the same %d != %d", score, job.Priority))
	}

	err = rjq.Cancel(job)
	if err != nil {
		t.Fatal(err)
	}

	// Test to make sure the Job data is no longer there and not in a queue.
	res, err := rjq.Client.Exists(job_key).Result()
	if res == true {
		t.Fatal("Job data still exists: " + job_key)
	}

	if jobExistsInZSet(rjq, job, kqueued) {
		t.Fatal("Job data still exists in queue: " + kqueued)
	}
}

func TestAddConsumeCancel(t *testing.T) {
	conn := makeConn()
	rjq := MakeQueue(NS, conn)

	job := generateTestJobs(1)[0]

	kqueued := MakeKey(rjq.Namespace, "QUEUED")
	kworking := MakeKey(rjq.Namespace, "WORKING")
	// job_key := MakeKey(rjq.Namespace, "JOBS", job.JobID)

	defer conn.Close()
	defer removeQueues(rjq)

	err := rjq.Add(job)
	if err != nil {
		t.Fatal(err)
	}

	job2, err := rjq.Consume()
	if err != nil {
		t.Fatal(err)
	}

	if jobExistsInZSet(rjq, job, kqueued) {
		t.Fatal("Job should not be in the QUEUED queue after consumption.")
	}

	if !jobExistsInZSet(rjq, job, kworking) {
		t.Fatal("Job should be in the WORKING queue, but is not.")
	}

	err = rjq.Cancel(job2)
	if err == nil {
		t.Fatal("Job cancelled but the cancel operation should have been rejected.")
	}
}

func TestAddConsumeFailCancel(t *testing.T) {
	// Add, consume, fail to cancel, fail, cancel
	conn := makeConn()
	rjq := MakeQueue(NS, conn)
	rjq.WorkerName = "worker"

	job := generateTestJobs(1)[0]

	// kscheduled := MakeKey(rjq.Namespace, "SCHEDULED")
	kfailed := MakeKey(rjq.Namespace, "FAILED")
	job_key := MakeKey(rjq.Namespace, "JOBS", job.JobID)

	defer conn.Close()
	defer removeQueues(rjq)

	err := rjq.Add(job)
	if err != nil {
		t.Fatal(err)
	}

	job2, err := rjq.Consume()
	if err != nil {
		t.Fatal(err)
	}

	err = rjq.Fail(job2, 3600)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure it's on the FAILED queue.
	if !jobExistsInZSet(rjq, job2, kfailed) {
		printQueues(rjq)
		t.Fatal("Job should be in the FAILED queue after fail.")
	}

	err = rjq.Cancel(job2)
	if err != nil {
		printQueues(rjq)
		t.Fatal(err)
	}

	if jobExistsInAnyZSet(rjq, job2) {
		printQueues(rjq)
		t.Fatal("Cancel failed. Job still exists")
	}

	// Make sure the job data doesn't exist.
	exists, err := rjq.Client.Exists(job_key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists == true {
		t.Fatal("Job object still exists: %s", job_key)
	}
}

func TestRecover(t *testing.T) {
	conn := makeConn()
	rjq := MakeQueue(NS, conn)
	removeQueues(rjq)

	job := generateTestJobs(1)[0]
	job_key := MakeKey(rjq.Namespace, "JOBS", job.JobID)

	kworkers := MakeKey(rjq.Namespace, "WORKERS")
	kworker := MakeKey(kworkers, rjq.WorkerName)
	kworker_active := MakeKey(kworker, "ACTIVE")

	// Clean up
	defer conn.Close()
	defer removeQueues(rjq)
	defer rjq.Client.Del(job_key)

	// Make sure it doesn't exist yet before adding
	_ = rjq.Client.Del(job_key)

	err := rjq.Add(job)
	if err != nil {
		t.Fatal(err)
	}

	job2, err := rjq.Consume()
	if err != nil {
		fmt.Println(job2)
		t.Fatal("Consume failed.")
	}
	// Remove the active flag (Simulate worker expiration).
	rjq.Client.Del(kworker_active)

	res, err := rjq.Recover()
	if len(res) != 1 || res[0] != job.JobID {
		t.Fatal(fmt.Sprintf("Expected [%s], found: %v", job.JobID, res))
	}
	if err != nil {
		t.Fatal(err)
	}

	for _, jobid := range res {
		job3, err := rjq.Get(jobid)
		if err != nil {
			printQueues(rjq)
			t.Fatal(err)
		}
		if job3.Failures != 1 {
			printQueues(rjq)
			fmt.Println(job3)
			t.Fatal("Failure count incorrect:", job3.Failures)
		}
	}

}

func TestMaxFailed(t *testing.T) {
	conn := makeConn()
	rjq := MakeQueue(NS, conn)
	defer conn.Close()
	defer removeQueues(rjq)

	kmaxfailed := MakeKey(NS, "MAXFAILED")
	_ = rjq.Client.Del(kmaxfailed)
	maxfailed := 5
	n, err := rjq.SetMaxFailed(maxfailed)
	if err != nil {
		t.Fatal(err)
	}
	if n != maxfailed {
		t.Fatal(fmt.Sprintf("%d != %d", n, maxfailed))
	}
	_ = rjq.Client.Del(kmaxfailed)
}

func TestMaxJobs(t *testing.T) {
	conn := makeConn()
	rjq := MakeQueue(NS, conn)
	defer conn.Close()
	defer removeQueues(rjq)
	kmaxjobs := MakeKey(NS, "MAXJOBS")
	_ = rjq.Client.Del(kmaxjobs)
	maxjobs := 5
	n, err := rjq.SetMaxFailed(maxjobs)
	if err != nil {
		t.Fatal(err)
	}
	if n != maxjobs {
		t.Fatal(fmt.Sprintf("%d != %d", n, maxjobs))
	}
	_ = rjq.Client.Del(kmaxjobs)
}

func TestAdd(t *testing.T) {
	// 1. Establish Connection
	conn := makeConn()
	rjq := MakeQueue(NS, conn)
	defer conn.Close()
	defer removeQueues(rjq)

	// 2. Make and add Job
	job := generateTestJobs(1)[0]

	job_key := MakeKey(rjq.Namespace, "JOBS", job.JobID)

	// Make sure it doesn't exist yet before adding
	_ = rjq.Client.Del(job_key)

	kqueued := MakeKey(rjq.Namespace, "QUEUED")
	kworking := MakeKey(rjq.Namespace, "WORKING")
	kscheduled := MakeKey(rjq.Namespace, "SCHEDULED")
	kfailed := MakeKey(rjq.Namespace, "FAILED")
	// fmt.Println("kqueued: ", kqueued)
	// fmt.Println("kworking: ", kworking)
	_ = rjq.Client.ZRem(kqueued, job.JobID)
	_ = rjq.Client.ZRem(kworking, job.JobID)

	err := rjq.Add(job)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Test the stored job data directly in Redis
	job_arr, err := rjq.Client.HGetAll(job_key).Result()
	if err != nil {
		t.Fatal(err)
	}

	// 3b. Test conversion to Job object
	job2, err := JobFromStringArray(job_arr)
	if err != nil {
		t.Fatal(err)
	}
	if job2.State != "enqueued" {
		t.Fatal("State is not enqueued. State: ", job2.State)
	}
	// fmt.Println(job)
	// fmt.Println(job2)

	if !CompareJobs(job, job2) {
		t.Fatal("Job data doesn't match.")
	}

	// 4. Consume Job
	job3, err := rjq.Consume()
	// fmt.Println(job3)
	if err != nil {
		t.Fatal(err)
	}

	// 5. Verify Job Contents
	// if !reflect.DeepEqual(job, job3) {
	if !CompareJobs(job, job3) {
		t.Fatal("Job data doesn't match.")
	}
	// Check owner ID
	if job3.Owner != rjq.WorkerName {
		t.Fatal("Job Owner doesn't match WorkerName.")
	}

	// 6. Test that the Job is on the right queue
	res, err := rjq.Client.ZScore(kworking, job.JobID).Result()
	// fmt.Println("Job score", res)

	if res != job.Priority {
		t.Fatal("Job Priority does not match.", res)
	}

	// 7. Ack the Job
	err = rjq.Ack(job3)
	if err != nil {
		t.Fatal(err)
	}

	// 8. Test that the Job does not exist on any queue
	res, err = rjq.Client.ZScore(kqueued, job.JobID).Result()
	if res > 0 {
		t.Fatal("Job still in queue.", job.JobID, "Score", res)
	}
	res, err = rjq.Client.ZScore(kworking, job.JobID).Result()
	if res > 0 {
		t.Fatal("Job still in working queue.", job.JobID, "Score", res)
	}

	// Test fail case
	err = rjq.Add(job)
	if err != nil {
		t.Fatal(err)
	}
	err = rjq.Fail(job, 3600)
	job4, err := rjq.Consume()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(job4)
	res, err = rjq.Client.ZScore(kscheduled, job.JobID).Result()
	// should not be on the scheduled queue
	res, err = rjq.Client.ZScore(kfailed, job.JobID).Result()
	// should be on the failed queue
}
