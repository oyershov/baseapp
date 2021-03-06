package daemons

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	jwtgo "github.com/dgrijalva/jwt-go"
	"github.com/openware/kaigara/pkg/vault"
	"github.com/openware/sonic"
)

// LicenseResponse to store response from api
type LicenseResponse struct {
	License string `json:"license"`
	Expire  int64  `json:"expire"`
}

func parseLicense(lic, component string) (int64, int64, error) {
	s := strings.Split(lic, ".")
	if len(s) != 3 {
		return 0, 0, fmt.Errorf("Unexpected license format")
	}

	data, err := base64.RawURLEncoding.DecodeString(s[1])
	if err != nil {
		return 0, 0, err
	}

	license := make(map[string]interface{})
	json.Unmarshal(data, &license)

	c := license[component].(map[string]interface{})

	exp, err := c["expire"].(json.Number).Int64()
	if err != nil {
		return 0, 0, err
	}

	cre, err := c["creation"].(json.Number).Int64()
	if err != nil {
		return 0, 0, err
	}

	return exp, cre, nil
}

// LicenseRenewal to periodic check and renew license before expire
func LicenseRenewal(appName string, app *sonic.Runtime, vaultService *vault.Service) {
	for {
		for {
			lic, err := getLicenseFromVault(appName, vaultService)
			if err != nil {
				log.Println("License is not found in vault")
				break
			}

			expire, creation, err := parseLicense(lic, appName)
			if err != nil {
				log.Println(err.Error())
				break
			}

			// Check to skip renewal (less than 75% of expire time)
			if time.Now().Unix() < creation+((expire-creation)*75/100) {
				log.Println("License renewal was skipped")
				break
			}

			err = createNewLicense(appName, app, vaultService)
			if err != nil {
				log.Println(err.Error())
				break
			}
			log.Println("License was renewed")

			break
		}

		time.Sleep(time.Minute * 15) // FIXME: Adjust polling period
	}
}

func createNewLicense(appName string, app *sonic.Runtime, vaultService *vault.Service) error {
	platformID, err := getPlatformIDFromVault(vaultService)
	if err != nil {
		return err
	}

	opendaxConfig := app.Conf.Opendax
	url, err := url.Parse(opendaxConfig.Addr)
	if err != nil {
		return err
	}
	url.Path = path.Join(url.Path, "/api/v2/opx/sonic/licenses/new")

	privRaw, err := getPrivateKeyFromVault(vaultService)
	if err != nil {
		return err
	}

	privPEM, err := base64.StdEncoding.DecodeString(privRaw)
	if err != nil {
		return err
	}

	key, err := jwtgo.ParseRSAPrivateKeyFromPEM(privPEM)
	if err != nil {
		return err
	}
	t := jwtgo.New(jwtgo.GetSigningMethod("RS256"))
	jwtToken, err := t.SignedString(key)

	req, err := http.NewRequest(http.MethodPost, url.String(), nil)
	if err != nil {
		return err
	}
	// Add request header
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("PlatformID", platformID)
	req.Header.Add("Authorization", "Bearer "+jwtToken)

	// Call HTTP request
	httpClient := &http.Client{}
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Convert response body to []byte
	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	// Check for API error
	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("Unexpected status %d", res.StatusCode)
	}

	license := LicenseResponse{}
	err = json.Unmarshal(resBody, &license)
	if err != nil {
		return err
	}

	err = saveLicenseToVault(appName, vaultService, license.License)
	if err != nil {
		return err
	}

	return nil
}

func getPlatformIDFromVault(vaultService *vault.Service) (string, error) {
	app := "peatio"
	scope := "private"
	key := "platform_id"

	// Load secret
	vaultService.LoadSecrets(app, scope)

	// Get secret
	result, err := vaultService.GetSecret(app, key, scope)
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", fmt.Errorf("Kaigara config %s.%s.%s not found", app, scope, key)
	}

	return result.(string), nil
}

func getPrivateKeyFromVault(vaultService *vault.Service) (string, error) {
	app := "sonic"
	scope := "secret"
	key := "jwt_private_key"

	// Load secret
	vaultService.LoadSecrets(app, scope)

	// Get secret
	result, err := vaultService.GetSecret(app, key, scope)
	if err != nil {
		return "", err
	}

	return result.(string), nil
}

func getLicenseFromVault(app string, vaultService *vault.Service) (string, error) {
	scope := "secret"

	// Load secret
	vaultService.LoadSecrets(app, scope)

	// Get secret
	license, err := vaultService.GetSecret(app, "finex_license_key", scope)
	if err != nil {
		return "", err
	}

	return license.(string), nil
}

func saveLicenseToVault(app string, vaultService *vault.Service, license string) error {
	scope := "secret"

	// Load secret
	vaultService.LoadSecrets(app, scope)

	// Get secret
	err := vaultService.SetSecret(app, "finex_license_key", license, scope)
	if err != nil {
		return err
	}

	// Save secret
	err = vaultService.SaveSecrets(app, scope)
	if err != nil {
		return err
	}

	return nil
}
