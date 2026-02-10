package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
)

var _ = Describe("Integrations API", Ordered, Label("integration"), func() {
	var (
		client       uptimerobot.Client
		slackWebhook string
		createdID    int
	)

	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping Integrations API tests: UPTIME_ROBOT_API_KEY not set")
		}

		slackWebhook = os.Getenv("UPTIME_ROBOT_SLACK_WEBHOOK_URL")
		if slackWebhook == "" {
			Skip("Skipping Integrations API tests: UPTIME_ROBOT_SLACK_WEBHOOK_URL not set")
		}

		client = uptimerobot.NewClient(os.Getenv("UPTIME_ROBOT_API_KEY"))
	})

	AfterAll(func() {
		if createdID == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = client.DeleteIntegration(ctx, createdID)
	})

	It("should create a Slack integration via API", func() {
		friendlyName := fmt.Sprintf("e2e-slack-integration-%d", time.Now().UnixNano())
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		resp, err := client.CreateSlackIntegration(ctx, uptimerobot.SlackIntegrationData{
			FriendlyName:           friendlyName,
			EnableNotificationsFor: "Down",
			SSLExpirationReminder:  false,
			WebhookURL:             slackWebhook,
			CustomValue:            "created by e2e test",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.ID).To(BeNumerically(">", 0))
		createdID = resp.ID

		Eventually(func(g Gomega) {
			listCtx, listCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer listCancel()

			integrations, listErr := client.ListIntegrations(listCtx)
			g.Expect(listErr).NotTo(HaveOccurred())

			found := false
			for _, integration := range integrations {
				if integration.ID == createdID {
					found = true
					g.Expect(integration.Type).NotTo(BeNil())
					g.Expect(*integration.Type).To(Equal("Slack"))
					if integration.FriendlyName != nil {
						g.Expect(*integration.FriendlyName).To(Equal(friendlyName))
					}
					break
				}
			}
			g.Expect(found).To(BeTrue(), "expected integration id %d to be present in list", createdID)
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	})
})
