# 🤖 Fortmont Agent

The **Fortmont Agent** is a small application that runs on a computer or server and connects it to **Fortmont**.

Think of it as the **bridge between a device and Fortmont** 🌉. It allows Fortmont to securely communicate with and monitor devices without requiring those devices to be directly exposed to the internet.

## ✨ What does it do?

Once installed, the Fortmont Agent:

* 🔗 Connects the device to Fortmont.
* 🖥️ Identifies the device and reports basic information about it.
* 🔐 Maintains a secure connection with Fortmont.
* 💓 Lets Fortmont know that the device is online.
* 🔄 Automatically reconnects if the connection is interrupted.
* ⚙️ Can run quietly in the background as a system service.

The goal is to make connecting a device to Fortmont as simple as **installing the agent and registering it**.

## 🚀 Getting Started

When an agent is installed for the first time, Fortmont provides a **one-time enrollment token**.

The token securely registers the device with Fortmont.

After registration, the agent remembers its identity and **doesn't need the enrollment token again**.

For example:

```bash
fortmont-agent --token <enrollment-token>
```

Once setup is complete, the agent can run normally:

```bash
fortmont-agent
```

It can also be installed as a background service so it automatically starts with the computer.

## ⚙️ Running as a Service

Fortmont supports running the agent as a normal system service on:

* 🪟 Windows
* 🐧 Linux

For example:

```text
fortmont-agent service install --token <enrollment-token>
fortmont-agent service status
fortmont-agent service uninstall
```

This allows the agent to run quietly in the background without someone needing to start it manually.

## 🌐 Staying Connected

The agent can connect to multiple Fortmont servers.

If one server isn't available, the agent can automatically try another. If the connection is temporarily lost, it will keep trying to reconnect.

This helps Fortmont stay reliable even when there are temporary network problems or when one of its servers is unavailable. 💪

## 🔐 Security

Security is an important part of the Fortmont Agent.

The initial enrollment token is only used when the device is first registered. After that, the agent uses its own secure identity to prove that it is the registered device.

🔒 Private credentials stay on the device and are never sent to Fortmont.

Fortmont can also disable or revoke an agent if necessary. A revoked device won't be able to simply reconnect using its existing credentials.

## 💡 In Simple Terms

Think of the Fortmont Agent as the **connector between a device and Fortmont**:

```text
┌──────────────────┐
│   🖥️ Your Server  │
│                  │
│  🤖 Fortmont     │
│      Agent       │
└────────┬─────────┘
         │
         │ 🔐 Secure connection
         ▼
┌──────────────────┐
│ ☁️ Fortmont Cloud │
│                  │
│  Control Plane   │
└──────────────────┘
```

Install the agent, register the device, and Fortmont can securely communicate with it. 🚀

The agent is designed to stay **simple, reliable, and easy to use**, while taking care of the connection in the background.
