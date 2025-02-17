/*
 Licensed to the Apache Software Foundation (ASF) under one
 or more contributor license agreements.  See the NOTICE file
 distributed with this work for additional information
 regarding copyright ownership.  The ASF licenses this file
 to you under the Apache License, Version 2.0 (the
 "License"); you may not use this file except in compliance
 with the License.  You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package queuequotamgmt_test

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/apache/yunikorn-core/pkg/webservice/dao"
	"github.com/apache/yunikorn-k8shim/test/e2e/framework/helpers/common"
	"github.com/apache/yunikorn-k8shim/test/e2e/framework/helpers/k8s"
	"github.com/apache/yunikorn-k8shim/test/e2e/framework/helpers/yunikorn"

	v1 "k8s.io/api/core/v1"
)

var _ = Describe("", func() {
	var kClient k8s.KubeCtl
	var restClient yunikorn.RClient
	var err error
	var sleepRespPod *v1.Pod
	var maxCPU int64
	var maxMem int64
	var maxCPUAnnotation = "yunikorn.apache.org/namespace.max.cpu"
	var maxMemAnnotation = "yunikorn.apache.org/namespace.max.memory"
	var annotations map[string]string
	var ns string
	var queueInfo *dao.PartitionQueueDAOInfo
	var maxResource yunikorn.ResourceUsage
	var usedResource yunikorn.ResourceUsage
	var usedPerctResource yunikorn.ResourceUsage
	var pods []string

	BeforeEach(func() {
		// Initializing kubectl client
		kClient = k8s.KubeCtl{}
		Ω(kClient.SetClient()).To(BeNil())
		// Initializing rest client
		restClient = yunikorn.RClient{}
		annotations = map[string]string{}
		pods = []string{}
	})

	/*
		1. Create a namespace with annotations set to max CPU and max Mem
		2. Submit pod with CPU and Mem requests
		3. Verify the CPU and memory usage reported by Yunikorn is accurate.
		4. Submit another pod with resource request that exceeds the limits
		5. Verify the pod is in un-schedulable state
		6. Wait until pod-1 completes and verify that pod-2 is scheduled now.
	*/
	It("Verify_Queue_Quota_Allocation", func() {
		maxMem = 300
		maxCPU = 300
		annotations[maxCPUAnnotation] = strconv.FormatInt(maxCPU, 10) + "m"
		annotations[maxMemAnnotation] = strconv.FormatInt(maxMem, 10) + "M"

		ns = "ns-" + common.RandSeq(10)
		By(fmt.Sprintf("create %s namespace with maxCPU: %dm and maxMem: %dM", ns, maxCPU, maxMem))
		ns1, err1 := kClient.CreateNamespace(ns, annotations)
		Ω(err1).NotTo(HaveOccurred())
		Ω(ns1.Status.Phase).To(Equal(v1.NamespaceActive))

		// Create sleep pod template
		var reqCPU int64 = 100
		var reqMem int64 = 100

		var iter int64
		for iter = 1; iter <= 3; iter++ {
			sleepPodConfigs := k8s.SleepPodConfig{NS: ns, Time: 60, CPU: reqCPU, Mem: reqMem}
			sleepObj, podErr := k8s.InitSleepPod(sleepPodConfigs)
			Ω(podErr).NotTo(HaveOccurred())

			pods = append(pods, sleepObj.Name)
			By(fmt.Sprintf("App-%d: Deploy the sleep app:%s to %s namespace", iter, sleepObj.Name, ns))
			sleepRespPod, err = kClient.CreatePod(sleepObj, ns)
			Ω(err).NotTo(HaveOccurred())

			Ω(kClient.WaitForPodRunning(sleepRespPod.Namespace, sleepRespPod.Name, time.Duration(60)*time.Second)).NotTo(HaveOccurred())

			// Verify that the resources requested by above sleep pod is accounted for in the queues response
			queueInfo, err = restClient.GetSpecificQueueInfo("default", "root."+ns)
			Ω(err).NotTo(HaveOccurred())
			Ω(queueInfo).NotTo(BeNil())
			Ω(queueInfo.QueueName).Should(Equal("root." + ns))
			Ω(queueInfo.Status).Should(Equal("Active"))
			Ω(queueInfo.Properties).Should(BeEmpty())
			maxResource.ParseResourceUsage(queueInfo.MaxResource)
			usedResource.ParseResourceUsage(queueInfo.AllocatedResource)
			usedPerctResource.ParseResourceUsage(queueInfo.AbsUsedCapacity)

			By(fmt.Sprintf("App-%d: Verify max capacity on the queue is accurate", iter))
			Ω(maxResource.GetVCPU()).Should(Equal(maxCPU))
			Ω(maxResource.GetMemory()).Should(Equal(maxMem * 1000 * 1000))

			By(fmt.Sprintf("App-%d: Verify used capacity on the queue is accurate after 1st pod deployment", iter))
			Ω(usedResource.GetVCPU()).Should(Equal(reqCPU * iter))
			Ω(usedResource.GetMemory()).Should(Equal(reqMem * iter * 1000 * 1000))

			var perctCPU = int64(math.Floor((float64(reqCPU*iter) / float64(maxCPU)) * 100))
			var perctMem = int64(math.Floor((float64(reqMem*iter) / float64(maxMem)) * 100))
			By(fmt.Sprintf("App-%d: Verify used percentage capacity on the queue is accurate after 1st pod deployment", iter))
			Ω(usedPerctResource.GetVCPU()).Should(Equal(perctCPU))
			Ω(usedPerctResource.GetMemory()).Should(Equal(perctMem))
		}

		By("App-4: Submit another app which exceeds the queue quota limitation")
		sleepPodConfigs := k8s.SleepPodConfig{NS: ns, Time: 60, CPU: reqCPU, Mem: reqMem}
		sleepObj, app4PodErr := k8s.InitSleepPod(sleepPodConfigs)
		Ω(app4PodErr).NotTo(HaveOccurred())

		By(fmt.Sprintf("App-4: Deploy the sleep app:%s to %s namespace", sleepObj.Name, ns))
		sleepRespPod, err = kClient.CreatePod(sleepObj, ns)
		Ω(err).NotTo(HaveOccurred())
		pods = append(pods, sleepObj.Name)

		By(fmt.Sprintf("App-4: Verify app:%s in accepted state", sleepObj.Name))
		// Wait for pod to move to accepted state
		err = restClient.WaitForAppStateTransition("default", "root."+ns, sleepRespPod.ObjectMeta.Labels["applicationId"],
			yunikorn.States().Application.Accepted,
			120)
		Ω(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Pod-4: Verify pod:%s is in pending state", sleepObj.Name))
		err = kClient.WaitForPodPending(ns, sleepObj.Name, time.Duration(60)*time.Second)
		Ω(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("App-1: Wait for 1st app:%s to complete, to make enough capacity to run the last app", pods[0]))
		// Wait for pod to move to accepted state
		err = kClient.WaitForPodSucceeded(ns, pods[0], time.Duration(360)*time.Second)
		Ω(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Pod-4: Verify Pod:%s moved to running state", sleepObj.Name))
		Ω(kClient.WaitForPodRunning(sleepRespPod.Namespace, sleepRespPod.Name, time.Duration(60)*time.Second)).NotTo(HaveOccurred())

	}, 360)

	It("Verify_NS_Without_Quota", func() {
		ns = "ns-" + common.RandSeq(10)
		By(fmt.Sprintf("create %s namespace without quota information", ns))
		ns1, err1 := kClient.CreateNamespace(ns, nil)
		Ω(err1).NotTo(HaveOccurred())
		Ω(ns1.Status.Phase).To(Equal(v1.NamespaceActive))
	})

	DescribeTable("", func(x string, y string) {
		annotations[maxCPUAnnotation] = x
		annotations[maxMemAnnotation] = y
		ns = "ns-" + common.RandSeq(10)
		By(fmt.Sprintf("create %s namespace with empty annotations", ns))
		ns1, err1 := kClient.CreateNamespace(ns, annotations)
		Ω(err1).NotTo(HaveOccurred())
		Ω(ns1.Status.Phase).To(Equal(v1.NamespaceActive))

		// Create sleep pod
		var reqCPU int64 = 300
		var reqMem int64 = 300
		sleepPodConfigs := k8s.SleepPodConfig{NS: ns, Time: 60, CPU: reqCPU, Mem: reqMem}
		sleepObj, podErr := k8s.InitSleepPod(sleepPodConfigs)
		Ω(podErr).NotTo(HaveOccurred())
		By(fmt.Sprintf("Deploy the sleep app:%s to %s namespace", sleepObj.Name, ns))
		sleepRespPod, err = kClient.CreatePod(sleepObj, ns)
		Ω(err).NotTo(HaveOccurred())
		Ω(kClient.WaitForPodRunning(sleepRespPod.Namespace, sleepRespPod.Name, time.Duration(60)*time.Second)).NotTo(HaveOccurred())
	},
		Entry("Verify_NS_With_Empty_Quota_Annotations", nil, nil),
		Entry("Verify_NS_With_Wrong_Quota_Annotations", "a", "b"),
	)

	// Hierarchical Queues - Quota enforcement
	// For now, these cases are skipped
	PIt("Verify_Child_Max_Resource_Must_Be_Less_Than_Parent_Max", func() {
		// P2 case - addressed later
	})

	PIt("Verify_Sum_Of_All_Child_Resource_Must_Be_Less_Than_Parent_Max", func() {
		// P2 case - addressed later
	})

	AfterEach(func() {
		By("Check Yunikorn's health")
		checks, err := yunikorn.GetFailedHealthChecks()
		Ω(err).NotTo(HaveOccurred())
		Ω(checks).To(Equal(""), checks)

		By("Tearing down namespace: " + ns)
		err = kClient.TearDownNamespace(ns)
		Ω(err).NotTo(HaveOccurred())
	})
})
