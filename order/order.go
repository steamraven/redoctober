// Package order manages the bookkeeping and utilies required
// for users to create an 'order' meaning they have requested
// delegations for a certian resource.
//
// Copyright (c) 2016 CloudFlare, Inc.
package order

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/cloudflare/redoctober/hipchat"
)

const (
	NewOrder       = "%s has created an order for the label %s. requesting %d delegations for %s"
	NewOrderLink   = "@%s - https://%s?%s"
	OrderFulfilled = "%s has had order %s fulfilled."
	NewDelegation  = "%s has delegated the label %s to %s (per order %s) for %s"
)

type Order struct {
	Creator string
	Users   []string
	Num     string

	TimeRequested     time.Time
	DurationRequested time.Duration
	Delegated         int
	OwnersDelegated   []string
	Owners            []string
	Labels            []string
}

type OrderIndex struct {
	OrderFor string

	OrderId     string
	OrderOwners []string
}

// Orders represents a mapping of Order IDs to Orders. This structure
// is useful for looking up information about individual Orders and
// whether or not an order has been fulfilled. Orders that have been
// fulfilled will be removed from the structure.
type Orderer struct {
	Orders        map[string]Order
	Hipchat       hipchat.HipchatClient
	AlternateName string
}

func CreateOrder(name, orderNum string, time time.Time, duration time.Duration, adminsDelegated, contacts, users, labels []string, numDelegated int) (ord Order) {
	ord.Creator = name
	ord.Num = orderNum
	ord.Labels = labels
	ord.TimeRequested = time
	ord.DurationRequested = duration
	ord.OwnersDelegated = adminsDelegated
	ord.Owners = contacts
	ord.Delegated = numDelegated
	ord.Users = users
	return
}

func GenerateNum() (num string) {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewOrder will create a new map of Orders
func NewOrderer(hipchatClient hipchat.HipchatClient) (o Orderer) {
	o.Orders = make(map[string]Order)
	o.Hipchat = hipchatClient
	o.AlternateName = "HipchatName"
	return
}

// notify is a generic function for using a notifier, but it checks to make
// sure that there is a notifier available, since there won't always be.
func notify(o *Orderer, msg, color string) {
	o.Hipchat.Notify(msg, color)
}
func (o *Orderer) NotifyNewOrder(duration, orderNum string, names, labels []string, uses int, owners map[string]string) {
	labelList := ""
	for i, label := range labels {
		if i == 0 {
			labelList += label
		} else {
			// Never include spaces in something go URI encodes. Go will
			// add a + to the string, instead of a %20
			labelList += "," + label
		}
	}
	nameList := ""
	for i, name := range names {
		if i == 0 {
			nameList += name
		} else {
			// Never include spaces in something go URI encodes. Go will
			// add a + to the string, instead of a %20
			nameList += "," + name
		}
	}

	n := fmt.Sprintf(NewOrder, nameList, labelList, uses, duration)
	notify(o, n, hipchat.RedBackground)
	for owner, hipchatName := range owners {
		queryParams := url.Values{
			"delegator": {owner},
			"label":     {labelList},
			"duration":  {duration},
			"uses":      {strconv.Itoa(uses)},
			"ordernum":  {orderNum},
			"delegatee": {nameList},
		}.Encode()
		notify(o, fmt.Sprintf(NewOrderLink, hipchatName, o.Hipchat.RoHost, queryParams), hipchat.GreenBackground)
	}
}

func (o *Orderer) NotifyDelegation(delegator, delegatee, orderNum, duration string, labels []string) {
	labelList := ""
	for i, label := range labels {
		if i == 0 {
			labelList += label
		} else {
			labelList += ", " + label
		}
	}
	n := fmt.Sprintf(NewDelegation, delegator, labelList, delegatee, orderNum, duration)
	notify(o, n, hipchat.YellowBackground)
}
func (o *Orderer) NotifyOrderFulfilled(name, orderNum string) {
	n := fmt.Sprintf(OrderFulfilled, name, orderNum)
	notify(o, n, hipchat.PurpleBackground)
}

func (o *Orderer) FindOrder(user string, labels []string) (string, bool) {
	for key, order := range o.Orders {
		foundLabel := false
		foundUser := false
		for _, orderUser := range order.Users {
			if orderUser == user {
				foundUser = true
			}
		}
		if !foundUser {
			continue
		}
		for _, ol := range order.Labels {
			foundLabel = false
			for _, il := range labels {
				if il == ol {
					foundLabel = true
				}
			}
		}
		if !foundLabel {
			continue
		}
		return key, true
	}
	return "", false
}

func  intersect(a []string,b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return []string{"Any"};
	}
	x := []string{};
	for _, a1 := range a {
		for _, b1 := range b {
			if a1 == b1 {
				x=append(x,a1);
			}
		}
	}
	return  x;
}


func (o *Orderer) UpdateOrders(owner string, time string,  users []string, labels []string) (err error) {
	owners := []string{owner}
	for key, order := range o.Orders {
		common_owners := intersect(owners, order.Owners)
		common_users := intersect(users, order.Users)
		common_labels := intersect(labels, order.Labels)
		if len(common_owners) > 0 &&
		   len(common_users) > 0 &&
		   len(common_labels) > 0 {
			if len(order.OwnersDelegated) == 0 {
				order.OwnersDelegated = append(order.OwnersDelegated, owner)
				order.Delegated++
			} else {
				for _, delegated := range order.OwnersDelegated {
					if delegated == owner {
						continue
					}
					order.OwnersDelegated = append(order.OwnersDelegated, owner)
					order.Delegated++
				}
			}
			o.Orders[key] = order
			for _, delegatedUser := range common_users {
				o.NotifyDelegation(owner, delegatedUser, key, time, common_labels)
			}
		}
	}
	return nil
}

func (o *Orderer) FulfillOrders(user string, owners []string, labels []string) (err error) {
	users := []string{user}
	for key, order := range o.Orders {
		if len(intersect(owners, order.Owners))  == len(owners) &&
		   len(intersect(users, order.Users)) > 0 &&
		   len(intersect(labels, order.Labels)) > 0 {
			delete(o.Orders, key)
			o.NotifyOrderFulfilled(user, key)
		}
	}
	return nil
}
