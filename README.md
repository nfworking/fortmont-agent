# 🤖 Fortmont Agent

The **Fortmont Agent** is a lightweight application that runs on a server or computer and securely connects it to **Fortmont**.

Its main purpose is to act as a **secure bridge between Fortmont and local infrastructure**. 🌉

By default, the agent collects and sends basic information about the device it is running on. Over time, the agent will also support **plugins** that allow Fortmont to connect to and interact with different infrastructure platforms such as **Proxmox, UniFi, and other services**.

## ✨ What does it do?

The Fortmont Agent provides a simple way to connect infrastructure to Fortmont.

By default, it:

* 🖥️ Reports basic device information and metrics.
* 📊 Sends information such as system resource usage and device status.
* 🔗 Maintains a secure connection with Fortmont.
* 🔄 Automatically reconnects if the connection is interrupted.
* ⚙️ Can run quietly in the background as a system service.

As the agent develops, plugins will extend its capabilities.

For example:

* 🖥️ **Proxmox** — Connect Fortmont to virtual machines, containers, and hosts.
* 🌐 **UniFi** — Connect Fortmont to network equipment and network information.
* 🔌 **Other plugins** — Add support for additional infrastructure platforms without having to completely rebuild the agent.

This makes the agent flexible while keeping the core application simple.

## 🚀 Getting Started

When an agent is installed for the first time, Fortmont provides a **one-time enrollment token**.

The token securely registers the device with Fortmont.

After registration, the agent remembers its identity and **doesn't need the enrollment token again**.

For example:

```bash
fortmont-agent --token <enrollment-token>
```

Once registered, the agent can run normally:

```bash
fortmont-agent
```

It can also be installed as a background service so it automatically starts with the computer.

## ⚙️ Running as a Service

Fortmont supports running the agent as a system service on:

* 🪟 Windows
* 🐧 Linux
* 🍎 macOS (Coming Soon)

For example:

```text
fortmont-agent service install --token <enrollment-token>
fortmont-agent service status
fortmont-agent service uninstall
```

This allows the agent to run quietly in the background without needing to be manually started.

## 🌐 Staying Connected

The agent maintains a secure connection to Fortmont so that infrastructure information can be exchanged when needed.

If the connection is interrupted, the agent automatically attempts to reconnect. 🔄

The agent can also connect through multiple Fortmont connection servers, helping keep the connection available if one server becomes unavailable.

## 🧩 Plugins

One of the main goals of the Fortmont Agent is to make it **extensible**.

Instead of building every infrastructure integration directly into the agent, plugins can provide support for different platforms.

For example:

```text
                 🤖 Fortmont Agent
                         │
          ┌──────────────┼──────────────┐
          │              │              │
          ▼              ▼              ▼
     🖥️ Proxmox       🌐 UniFi       🔌 Other
       Plugin          Plugin        Plugins
```

This means the core agent can remain lightweight while new infrastructure integrations can be added over time. 🚀

## 🔐 Security

Security is an important part of the Fortmont Agent.

The initial enrollment token is only used when the device is first registered. After registration, the agent uses its own secure identity to connect to Fortmont.

🔒 Private credentials remain on the infrastructure where they are stored and are not unnecessarily exposed to Fortmont.

Fortmont can also revoke an agent if access needs to be removed.

## 💡 In Simple Terms

Think of the Fortmont Agent as a **secure connector for infrastructure**.

It runs on a server or computer, connects that infrastructure to Fortmont, and provides basic device metrics out of the box.

As plugins are added, it can become a gateway to other infrastructure platforms too.

```text
       🖥️ Infrastructure
              │
              │
        🤖 Fortmont Agent
              │
       ┌──────┴──────┐
       │             │
   📊 Metrics     🧩 Plugins
                     │
              ┌──────┼──────┐
              ▼      ▼      ▼
           Proxmox  UniFi  Other
              │      │      │
              └──────┴──────┘
                     │
                     ▼
              ☁️ Fortmont
```

The goal is to make connecting infrastructure to Fortmont **simple, secure, and extensible**. 
