# v1 logger contracts

The interface-returning factories, `Logger` global and fluent methods introduced
in v1.17 remain unchanged. `LoggerItf` and custom implementations continue to work,
including replacing the interface-valued global default. This change does not
silently restore the incompatible pre-v1.17 signatures under the same names.

Consumers that require `*LoggerType` can opt into `NewConcreteLogger`,
`NewConcreteLoggerWithName`, `NewConcreteConsoleLoggerWithName`, and
`CreateNewDefaultConcreteLogger`. Concrete chains use `CloneConcrete`,
`NamedConcrete`, `WithConcrete`, and `WithOptionsConcrete`. These methods share
the same construction, Zap logger and dynamic-level behavior as their interface
counterparts. No type assertion, reflection or interface unwrapping is required.
`LoggerConfig` and `LoggerOption` are additive aliases for the former public option
names; the existing `LoggerOptFunc` remains available.

The historical v1.16-to-v1.17 return-type change was source-incompatible. Restoring
the old same-name concrete signatures now would in turn break valid v1.17 interface
assignments. Compile controls cover both sides; the rejected restoration prototype
is not the shipped design. The additive concrete entry points avoid a second
breaking change. Legacy callers that require concrete values must intentionally
adopt these entry points after an appropriate dependency version is available.

No removed company-specific notification API is restored, and no published tag
is rewritten or new version published. Local module replacements qualify source
candidates only. Downstream use of the new entry points still requires dependency
adoption; a local passing test is not a released module.
