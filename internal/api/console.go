package api

import (
	"fmt"
)

const (
	consoleMetricsPath  = "/dashboard/metrics"
	walletBalancePath   = "/dashboard/wallet/balance"
	consoleMessagesPath = "/dashboard/messages"
)

func (client *Client) GetMetrics(mode string) (map[string]any, error) {
	path := fmt.Sprintf("%s?mode=%s", consoleMetricsPath, mode)

	var metrics map[string]any
	if getError := client.Get(path, &metrics); getError != nil {
		return nil, fmt.Errorf("failed to fetch console metrics: %w", getError)
	}

	return metrics, nil
}

func (client *Client) GetWalletBalance(mode string) (map[string]any, error) {
	path := fmt.Sprintf("%s?mode=%s", walletBalancePath, mode)

	var balance map[string]any
	if getError := client.Get(path, &balance); getError != nil {
		return nil, fmt.Errorf("failed to fetch wallet balance: %w", getError)
	}

	return balance, nil
}

func (client *Client) GetMessages(mode string, page int, size int) (map[string]any, error) {
	path := fmt.Sprintf("%s?mode=%s&page=%d&size=%d", consoleMessagesPath, mode, page, size)

	var messages map[string]any
	if getError := client.Get(path, &messages); getError != nil {
		return nil, fmt.Errorf("failed to fetch console messages: %w", getError)
	}

	return messages, nil
}
