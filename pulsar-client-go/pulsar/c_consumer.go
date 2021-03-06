//
// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.
//

package pulsar

/*
#include "c_go_pulsar.h"
*/
import "C"

import (
	"runtime"
	"time"
	"unsafe"
	"context"
)

type consumer struct {
	ptr            *C.pulsar_consumer_t
	defaultChannel chan ConsumerMessage
}

func consumerFinalizer(c *consumer) {
	if c.ptr != nil {
		C.pulsar_consumer_free(c.ptr)
	}
}

//export pulsarSubscribeCallbackProxy
func pulsarSubscribeCallbackProxy(res C.pulsar_result, ptr *C.pulsar_consumer_t, ctx unsafe.Pointer) {
	cc := restorePointer(ctx).(*subscribeContext)

	C.pulsar_consumer_configuration_free(cc.conf)

	if res != C.pulsar_result_Ok {
		cc.callback(nil, newError(res, "Failed to subscribe to topic"))
	} else {
		cc.consumer.ptr = ptr
		runtime.SetFinalizer(cc.consumer, consumerFinalizer)
		cc.callback(cc.consumer, nil)
	}
}

type subscribeContext struct {
	conf     *C.pulsar_consumer_configuration_t
	consumer *consumer
	callback func(Consumer, error)
}

func subscribeAsync(client *client, options ConsumerOptions, callback func(Consumer, error)) {
	if options.Topic == "" {
		go callback(nil, newError(C.pulsar_result_InvalidConfiguration, "topic is required"))
		return
	}

	if options.SubscriptionName == "" {
		go callback(nil, newError(C.pulsar_result_InvalidConfiguration, "subscription name is required"))
		return
	}

	conf := C.pulsar_consumer_configuration_create()

	consumer := &consumer{}

	if options.MessageChannel == nil {
		// If there is no message listener, set a default channel so that we can have receive to
		// use that
		consumer.defaultChannel = make(chan ConsumerMessage)
		options.MessageChannel = consumer.defaultChannel
	}

	C._pulsar_consumer_configuration_set_message_listener(conf, savePointer(&consumerCallback{
		consumer: consumer,
		channel:  options.MessageChannel,
	}))

	if options.AckTimeout != 0 {
		timeoutMillis := options.AckTimeout.Nanoseconds() / int64(time.Millisecond)
		C.pulsar_consumer_set_unacked_messages_timeout_ms(conf, C.ulonglong(timeoutMillis))
	}

	if options.Type != Exclusive {
		C.pulsar_consumer_configuration_set_consumer_type(conf, C.pulsar_consumer_type(options.Type))
	}

	// ReceiverQueueSize==0 means to use the default queue size
	// -1 means to disable the consumer prefetching
	if options.ReceiverQueueSize > 0 {
		C.pulsar_consumer_configuration_set_receiver_queue_size(conf, C.int(options.ReceiverQueueSize))
	} else if options.ReceiverQueueSize < 0 {
		// In C++ client lib, 0 means disable prefetching
		C.pulsar_consumer_configuration_set_receiver_queue_size(conf, C.int(0))
	}

	if options.MaxTotalReceiverQueueSizeAcrossPartitions != 0 {
		C.pulsar_consumer_set_max_total_receiver_queue_size_across_partitions(conf,
			C.int(options.MaxTotalReceiverQueueSizeAcrossPartitions))
	}

	if options.Name != "" {
		name := C.CString(options.Name)
		defer C.free(unsafe.Pointer(name))

		C.pulsar_consumer_set_consumer_name(conf, name)
	}

	topic := C.CString(options.Topic)
	subName := C.CString(options.SubscriptionName)
	defer C.free(unsafe.Pointer(topic))
	defer C.free(unsafe.Pointer(subName))
	C._pulsar_client_subscribe_async(client.ptr, topic, subName,
		conf, savePointer(&subscribeContext{conf: conf, consumer: consumer, callback: callback}))
}

type consumerCallback struct {
	consumer Consumer
	channel  chan ConsumerMessage
}

//export pulsarMessageListenerProxy
func pulsarMessageListenerProxy(cConsumer *C.pulsar_consumer_t, message *C.pulsar_message_t, ctx unsafe.Pointer) {
	cc := restorePointerNoDelete(ctx).(*consumerCallback)

	defer func() {
		ex := recover()
		if ex != nil {
			// There was an error when sending channel (eg: already closed)
		}
	}()

	cc.channel <- ConsumerMessage{cc.consumer, newMessageWrapper(message)}
}

//// Consumer

func (c *consumer) Topic() string {
	return C.GoString(C.pulsar_consumer_get_topic(c.ptr))
}

func (c *consumer) Subscription() string {
	return C.GoString(C.pulsar_consumer_get_subscription_name(c.ptr))
}

func (c *consumer) Unsubscribe() error {
	channel := make(chan error)
	c.UnsubscribeAsync(func(err error) {
		channel <- err; close(channel) })
	return <-channel
}

func (c *consumer) UnsubscribeAsync(callback func(error)) {
	C._pulsar_consumer_unsubscribe_async(c.ptr, savePointer(callback))
}

//export pulsarConsumerUnsubscribeCallbackProxy
func pulsarConsumerUnsubscribeCallbackProxy(res C.pulsar_result, ctx unsafe.Pointer) {
	callback := restorePointer(ctx).(func(err error))

	if res != C.pulsar_result_Ok {
		go callback(newError(res, "Failed to unsubscribe consumer"))
	} else {
		go callback(nil)
	}
}

func (c *consumer) Receive(ctx context.Context) (Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case cm := <-c.defaultChannel:
		return cm.Message, nil
	}
}

func (c *consumer) Ack(msg Message) error {
	C.pulsar_consumer_acknowledge_async(c.ptr, msg.(*message).ptr, nil, nil)
	return nil
}

func (c *consumer) AckID(msgId MessageID) error {
	C.pulsar_consumer_acknowledge_async_id(c.ptr, msgId.(*messageID).ptr, nil, nil)
	return nil
}

func (c *consumer) AckCumulative(msg Message) error {
	C.pulsar_consumer_acknowledge_cumulative_async(c.ptr, msg.(*message).ptr, nil, nil)
	return nil
}

func (c *consumer) AckCumulativeID(msgId MessageID) error {
	C.pulsar_consumer_acknowledge_cumulative_async_id(c.ptr, msgId.(*messageID).ptr, nil, nil)
	return nil
}

func (c *consumer) Close() error {
	channel := make(chan error)
	c.CloseAsync(func(err error) { channel <- err; close(channel) })
	return <-channel
}

func (c *consumer) CloseAsync(callback func(error)) {
	if c.defaultChannel != nil {
		close(c.defaultChannel)
	}

	C._pulsar_consumer_close_async(c.ptr, savePointer(callback))
}

//export pulsarConsumerCloseCallbackProxy
func pulsarConsumerCloseCallbackProxy(res C.pulsar_result, ctx unsafe.Pointer) {
	callback := restorePointer(ctx).(func(err error))

	if res != C.pulsar_result_Ok {
		go callback(newError(res, "Failed to close Consumer"))
	} else {
		go callback(nil)
	}
}

func (c *consumer) RedeliverUnackedMessages() {
	C.pulsar_consumer_redeliver_unacknowledged_messages(c.ptr)
}
