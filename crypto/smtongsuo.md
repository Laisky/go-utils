# TongSuo

- <https://github.com/Tongsuo-Project/Tongsuo>

由阿里开源，有商用密码认证证书，在阿里产品上被广泛使用的国密算法库，基于 openssl 开发。


## Build

```sh
git clone https://github.com/Tongsuo-Project/Tongsuo.git

./config --prefix=/opt/tongsuo --libdir=/opt/tongsuo/lib enable-ntls
make -j
sudo make install

export LD_LIBRARY_PATH=/opt/tongsuo/lib
export PATH=$PATH:/opt/tongsuo/bin
```

## All in one

```sh
# root ca
tongsuo ecparam -genkey -name SM2 -out rootca.key
tongsuo req -out rootca.crt -outform PEM -key rootca.key \
    -set_serial 123456 \
    -days 3650 -x509 -new -nodes -utf8 -batch \
    -sm3 \
    -copy_extensions copyall \
    -extensions v3_ca \
    -config rootca.cnf

# intermedia
tongsuo ecparam -genkey -name SM2 -out intermediateca.key
tongsuo req -new -out intermediateca.csr -outform PEM \
    -key intermediateca.key \
    -sm3 \
    -config intermediateca.cnf -extensions v3_ca
tongsuo x509 -req -in intermediateca.csr -out intermediateca.crt -outform PEM \
    -CA rootca.crt -CAkey rootca.key -CAcreateserial \
    -days 3650 -utf8 -batch \
    -sm3 \
    -copy_extensions copyall \
    -extfile intermediateca.cnf -extensions v3_ca
tongsuo verify -x509_strict -CAfile rootca.crt intermediateca.crt

# leaf cert
tongsuo genpkey -out leaf.key -outform PEM -algorithm SM2
tongsuo req -new -out leaf.csr -outform PEM \
    -key leaf.key \
    -sm3 \
    -config leaf.cnf
tongsuo x509 -req -in leaf.csr -out leaf.crt -outform PEM \
    -CA intermediateca.crt -CAkey intermediateca.key -CAcreateserial \
    -days 3650 -utf8 -batch \
    -sm3 \
    -copy_extensions copyall \
    -extfile leaf.cnf -extensions v3_ca
tongsuo verify -x509_strict -CAfile rootca.crt -untrusted intermediateca.crt leaf.crt

cat leaf.crt intermediateca.crt rootca.crt > chain.crt
```

## Create RootCA

### Generate prikey

```sh
tongsuo ecparam -genkey -name SM2 -out rootca.key
```

### Show prikey info

```sh
tongsuo pkey -in rootca.key -text -noout
Private-Key: (256 bit)
priv:
    6d:a0:6f:00:9c:64:fd:89:10:87:ce:99:f6:a3:a9:
    38:69:58:21:3c:82:37:11:88:96:6c:5e:93:df:b8:
    16:b8
pub:
    04:83:fe:e4:10:2b:50:ce:c4:59:96:a1:2b:8b:86:
    41:4b:e5:61:74:22:89:0a:3b:30:7d:19:34:4e:54:
    cc:07:f0:81:05:72:80:26:1a:f5:fc:59:d4:8f:6a:
    d5:3d:42:94:3d:e3:12:9f:e1:d7:74:d3:2c:47:bf:
    d1:b5:24:b0:4b
ASN1 OID: SM2
```

### Generate root ca

```sh
tongsuo req -out rootca.crt -outform PEM -key rootca.key \
    -set_serial 123456 \
    -days 3650 -x509 -new -nodes -utf8 -batch \
    -sm3 \
    -copy_extensions copyall \
    -extensions v3_ca \
    -config rootca.cnf
```

{{< details "rootca.cnf" >}}

```ini
[ req ]
distinguished_name = req_distinguished_name
prompt = no
string_mask = utf8only
x509_extensions = v3_ca

[ req_distinguished_name ]
countryName = CN
stateOrProvinceName = Shanghai
localityName = Shanghai
organizationName = BBT
organizationalUnitName = XSS
commonName = Root CA

[ v3_ca ]
basicConstraints = critical, CA:TRUE
keyUsage = cRLSign, keyCertSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always, issuer
certificatePolicies = @pol_section-1, @pol_section-2

[ pol_section-1 ]
policyIdentifier = 1.3.6.1.4.1.59936.2.1
[ pol_section-2 ]
policyIdentifier = 1.3.6.1.4.1.59936.1.1.3

```

{{</ details >}}

### Show rootca info

tongsuo x509 -in rootca.crt -text -noout

```sh
Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            1e:20:62:5d:c2:62:d6:31:a8:15:23:ee:a1:6d:a8:b8:0a:c9:6e:88
        Signature Algorithm: 1.2.156.10197.1.501
        Issuer: C = CN, ST = Shanghai, L = Shanghai, O = BBT, CN = Root CA
        Validity
            Not Before: Dec 15 06:41:55 2023 GMT
            Not After : Dec 12 06:41:55 2033 GMT
        Subject: C = CN, ST = Shanghai, L = Shanghai, O = BBT, CN = Root CA
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:2f:bd:50:25:2b:f7:b0:ee:18:af:21:27:b2:71:
                    57:19:17:ec:25:78:c8:77:c6:09:4c:84:67:8a:52:
                    37:8c:46:da:ec:5c:6b:ed:d8:8b:19:c0:9d:d8:5d:
                    c2:9b:c8:65:5c:15:64:23:86:90:fb:85:de:8b:aa:
                    6b:af:95:c1:b9
                ASN1 OID: SM2
        X509v3 extensions:
            X509v3 Basic Constraints: critical
                CA:TRUE
            X509v3 Key Usage:
                Certificate Sign, CRL Sign
            X509v3 Subject Key Identifier:
                39:AE:5A:C6:01:54:0A:54:77:E1:C1:A4:EC:F1:FE:94:62:09:0B:B7
            X509v3 Authority Key Identifier:
                keyid:39:AE:5A:C6:01:54:0A:54:77:E1:C1:A4:EC:F1:FE:94:62:09:0B:B7

            X509v3 Certificate Policies:
                Policy: 1.3.6.1.4.1.59936.2.1
                Policy: 1.3.6.1.4.1.59936.1.1.3

    Signature Algorithm: 1.2.156.10197.1.501
         30:44:02:20:02:8b:1d:71:65:e1:f0:8b:a4:0b:de:b5:76:44:
         b1:ef:4b:79:62:52:29:72:f5:79:1f:2f:e6:a2:df:94:aa:a5:
         02:20:40:59:43:3a:e3:48:9e:e7:2c:39:cb:e3:c2:f4:cc:7d:
         24:63:5d:9e:87:ec:fe:08:01:2e:5a:89:56:e1:11:d9
```

## Create IntermediateCA

### generate prikey

```sh
# tongsuo genpkey -out intermediateca.key -outform PEM -algorithm SM2
tongsuo ecparam -genkey -name SM2 -out intermediateca.key
```

### Generate csr

```sh
tongsuo req -new -out intermediateca.csr -outform PEM \
    -key intermediateca.key \
    -sm3 \
    -config intermediateca.cnf -extensions v3_ca
```

{{< details "intermediate.cnf" >}}

```ini
[ req ]
distinguished_name = req_distinguished_name
prompt = no
string_mask = utf8only
x509_extensions = v3_ca
req_extensions = req_ext

[ req_ext ]
subjectAltName = @alt_names

[ req_distinguished_name ]
countryName = CN
stateOrProvinceName = Shanghai
localityName = Shanghai
organizationName = BBT
organizationalUnitName = XSS
commonName = Intermedia CA

[ v3_ca ]
basicConstraints = critical, CA:TRUE
keyUsage = cRLSign, keyCertSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always, issuer
certificatePolicies = @pol_section-1
subjectAltName = @alt_names

[ pol_section-1 ]
policyIdentifier = 1.2.3.4.5.6.7.8.1

[ alt_names ]
DNS.1 = localhost
DNS.2 = example.com

```

{{</ details >}}

### Verify CSR

```sh
tongsuo req -verify -in intermediateca.csr -noout

Certificate request self-signature verify OK

# check whether pukey is match
openssl ec -in intermediateca.key -pubout
openssl req -in intermediateca.csr -noout -pubkey

# show csr info
tongsuo req -in intermediateca.csr -text -noout
Certificate Request:
    Data:
        Version: 1 (0x0)
        Subject: C = CN, ST = Shanghai, L = Changning, O = BBT, CN = Intermediate CA
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:94:a6:d8:b1:39:8f:3b:38:97:2d:64:71:43:d4:
                    0d:3d:c9:6d:0a:11:b3:f8:82:36:92:0f:15:d7:54:
                    66:fb:09:d1:17:e6:78:11:4b:e7:b2:30:c5:87:09:
                    85:5f:37:95:0c:96:9d:c9:00:e7:47:44:38:11:8a:
                    c9:e2:95:6a:23
                ASN1 OID: SM2
        Attributes:
        Requested Extensions:
            X509v3 Subject Alternative Name:
                DNS:localhost, DNS:example.com, URI:https://localhost, email:sample@laisky.com
    Signature Algorithm: 1.2.156.10197.1.501
         30:44:02:20:5e:ab:0c:85:14:f0:02:5b:78:93:3e:61:85:2e:
         fe:f0:74:fd:b2:66:58:b7:48:63:f4:d0:48:73:2e:a4:aa:f0:
         02:20:74:59:bb:bb:c2:b6:43:9d:11:44:d3:65:32:bd:b2:e8:
         0b:f3:ba:fb:b8:cc:96:8e:c3:83:36:08:45:32:d4:80
```

### Sign intermediateca csr by rootca

```sh
tongsuo x509 -req -in intermediateca.csr -out intermediateca.crt -outform PEM \
    -CA rootca.crt -CAkey rootca.key -CAcreateserial \
    -days 3650 -utf8 -batch \
    -sm3 \
    -copy_extensions copyall \
    -extfile intermediateca.cnf -extensions v3_ca
```

### Verify intermediateca by rootca

```sh
tongsuo verify -x509_strict -CAfile rootca.crt intermediateca.crt

intermediateca.crt: OK
```

### Show intermediateca info

```sh
tongsuo x509 -in intermediateca.crt -text -noout
Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            37:db:05:eb:03:7b:e1:2e:8d:05:f5:5a:c0:3e:6c:ea:8e:4f:f0:26
        Signature Algorithm: 1.2.156.10197.1.501
        Issuer: C = CN, ST = Shanghai, L = Shanghai, O = BBT, OU = XSS, CN = Root CA
        Validity
            Not Before: Dec 15 09:19:25 2023 GMT
            Not After : Dec 12 09:19:25 2033 GMT
        Subject: C = CN, ST = Shanghai, L = Shanghai, O = BBT, OU = XSS, CN = Root CA
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:94:a6:d8:b1:39:8f:3b:38:97:2d:64:71:43:d4:
                    0d:3d:c9:6d:0a:11:b3:f8:82:36:92:0f:15:d7:54:
                    66:fb:09:d1:17:e6:78:11:4b:e7:b2:30:c5:87:09:
                    85:5f:37:95:0c:96:9d:c9:00:e7:47:44:38:11:8a:
                    c9:e2:95:6a:23
                ASN1 OID: SM2
        X509v3 extensions:
            X509v3 Basic Constraints: critical
                CA:TRUE
            X509v3 Key Usage:
                Certificate Sign, CRL Sign
            X509v3 Subject Key Identifier:
                E9:A6:FC:4B:95:8F:0C:D8:1A:DE:D1:1F:11:C7:C3:BC:45:62:9D:20
            X509v3 Authority Key Identifier:
                keyid:39:AE:5A:C6:01:54:0A:54:77:E1:C1:A4:EC:F1:FE:94:62:09:0B:B7

            X509v3 Certificate Policies:
                Policy: 1.2.3.4.5.6.7.8.1

    Signature Algorithm: 1.2.156.10197.1.501
         30:45:02:21:00:fa:d8:d3:29:f1:a1:30:49:de:e0:04:4f:f3:
         cb:9e:09:05:45:97:0a:83:8e:e5:ad:f7:98:5d:62:f2:65:6c:
         d0:02:20:46:ce:2d:a4:df:0c:51:1c:0a:98:11:8b:24:de:f2:
         61:8e:d6:12:4a:cc:6a:8d:6e:59:ed:c3:69:c2:a7:8d:4b
```

## Create Leaf Cert

### Generate prikey

```sh
tongsuo genpkey -out leaf.key -outform PEM -algorithm SM2
```

### Generate csr

```sh
tongsuo req -new -out leaf.csr -outform PEM \
    -key leaf.key \
    -sm3 \
    -config leaf.cnf
```

{{< details "leaf.cnf" >}}

```ini
[ req ]
distinguished_name = req_distinguished_name
prompt = no
string_mask = utf8only
x509_extensions = v3_ca

[ req_distinguished_name ]
countryName = CN
stateOrProvinceName = Shanghai
localityName = Shanghai
organizationName = BBT
organizationalUnitName = XSS
commonName = Leaf Cert

[ v3_ca ]
basicConstraints = critical, CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment, keyAgreement
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always, issuer
certificatePolicies = @pol_section-1

[ pol_section-1 ]
policyIdentifier = 1.2.3.4.5.6.7.8.1

```

{{</ details >}}

### Verify csr

```sh
tongsuo req -verify -in leaf.csr -noout
Certificate request self-signature verify OK
```

### Show csr info

```sh
tongsuo req -in leaf.csr -text -noout
Certificate Request:
    Data:
        Version: 1 (0x0)
        Subject: C = CN, ST = Shanghai, L = Shanghai, O = BBT, OU = XSS, CN = Leaf Cert
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:3f:81:28:7c:d3:09:e9:6a:e2:40:10:20:6a:b8:
                    29:e2:1c:d5:fd:98:33:25:e5:12:95:c2:aa:29:59:
                    2f:b9:48:dd:dc:30:50:c1:a8:15:a4:53:84:fc:94:
                    c5:44:3b:ce:6e:b6:51:e8:d7:f4:03:3c:a2:df:4c:
                    52:94:c9:05:c2
                ASN1 OID: SM2
        Attributes:
            a0:00
    Signature Algorithm: 1.2.156.10197.1.501
         30:46:02:21:00:f2:2b:2d:1c:f3:9f:2d:59:b6:42:35:cb:7a:
         ff:06:80:48:f5:86:f2:10:98:43:26:25:95:d8:cc:7a:f8:ec:
         95:02:21:00:c0:d7:33:ab:ea:9a:9c:46:8c:92:50:12:a7:12:
         f7:9f:33:6d:81:ff:bd:26:59:bc:4b:85:8e:de:6c:98:a3:03
```

### Sign leaf csr by intermediateca

```sh
tongsuo x509 -req -in leaf.csr -out leaf.crt -outform PEM \
    -CA intermediateca.crt -CAkey intermediateca.key -CAcreateserial \
    -days 3650 -utf8 -batch \
    -sm3 \
    -copy_extensions copyall \
    -extfile leaf.cnf -extensions v3_ca
```

### Show leaf info

```sh
tongsuo x509 -in leaf.crt -text -noout

Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            7f:dd:66:a0:64:59:97:0a:8a:d0:87:b6:3e:4c:83:e8:47:e7:c3:f1
        Signature Algorithm: 1.2.156.10197.1.501
        Issuer: C = CN, ST = Shanghai, L = Shanghai, O = BBT, OU = XSS, CN = Root CA
        Validity
            Not Before: Dec 15 09:22:30 2023 GMT
            Not After : Dec 12 09:22:30 2033 GMT
        Subject: C = CN, ST = Shanghai, L = Shanghai, O = BBT, OU = XSS, CN = Leaf Cert
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:3f:81:28:7c:d3:09:e9:6a:e2:40:10:20:6a:b8:
                    29:e2:1c:d5:fd:98:33:25:e5:12:95:c2:aa:29:59:
                    2f:b9:48:dd:dc:30:50:c1:a8:15:a4:53:84:fc:94:
                    c5:44:3b:ce:6e:b6:51:e8:d7:f4:03:3c:a2:df:4c:
                    52:94:c9:05:c2
                ASN1 OID: SM2
        X509v3 extensions:
            X509v3 Basic Constraints: critical
                CA:FALSE
            X509v3 Key Usage:
                Digital Signature, Non Repudiation, Key Encipherment, Data Encipherment, Key Agreement
            X509v3 Subject Key Identifier:
                E7:C5:74:6F:3E:25:A5:9A:64:2D:95:CB:08:66:D7:63:AD:C9:FF:8E
            X509v3 Authority Key Identifier:
                keyid:E9:A6:FC:4B:95:8F:0C:D8:1A:DE:D1:1F:11:C7:C3:BC:45:62:9D:20

            X509v3 Certificate Policies:
                Policy: 1.2.3.4.5.6.7.8.1

    Signature Algorithm: 1.2.156.10197.1.501
         30:45:02:20:3e:d3:e1:1a:a8:73:28:55:d8:46:94:aa:3f:65:
         17:f8:b4:7b:31:ea:20:57:d8:4f:06:2c:af:97:92:33:3c:bb:
         02:21:00:80:02:0e:59:3c:e3:4a:d1:c1:5d:c4:d1:31:62:80:
         b7:b7:bd:44:c2:07:be:81:5d:64:b0:ee:c3:4d:97:b8:d4
```

### Verify Leaf Cert By RootCA

```sh
tongsuo verify -x509_strict -CAfile rootca.crt -untrusted intermediateca.crt leaf.crt
```
