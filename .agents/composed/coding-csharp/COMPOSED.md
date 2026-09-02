---
name: coding-csharp
description: C# / .NET umbrella skill. Kai's first-class C# is Eco game modding against Eco.ReferenceAssemblies. Triggers - c#, csharp, .cs, .csproj, dotnet, nuget, msbuild, eco mod, EcoModKit.
seed:
  kind: language
  language: csharp
  extensions: [".cs"]
---

# coding-csharp

Umbrella for any C# / .NET work.

## Kai's actual C#

Kai has put substantial first-class time into C#. Do not treat her as a C# beginner. The bulk of it is Eco game modding - [`eco-mods`](https://github.com/coilyco-bridge/eco-mods) (private) and [`eco-mods-public`](https://github.com/coilyco-flight-deck/eco-mods-public) - compiled against `Eco.ReferenceAssemblies`. That work is attribute-heavy, partial-class-heavy gameplay code: items, skills, recipes, plant definitions wired into the Eco TechTree.

Eco-specific modding mechanics live in the `gaming-eco-*` skill family. This skill is the general C# layer underneath that.

## Defaults

- **Runtime**: modern .NET (`net10.0` in the Eco repos). Use the `TargetFramework` the project pins.
- **Project style**: SDK-style `.csproj`. `ImplicitUsings` enabled, `Nullable` enabled - both on in the Eco repos, keep them on for new projects.
- **Build/run**: `dotnet build`, `dotnet run`, `dotnet test`. On Kai's hosts these route through `ward` (the lockdown denies bare `dotnet`).
- **Packages**: NuGet via `<PackageReference>`. No `packages.config`.
- **Format**: `dotnet format`. Don't impose a custom `.editorconfig` on a project that has one.

## Style

- Nullable reference types on. Annotate intent, don't blanket-suppress with `!`.
- `var` when the type is obvious from the right-hand side, explicit type when it aids the reader.
- Expression-bodied members for one-liners (the Eco mods lean on this for `IDynamicValue` overrides).
- Match the project's `using` placement. The Eco repos put `using` directives inside the `namespace` block - follow the file you're in instead of reformatting.
- Async all the way down for I/O. No `.Result` / `.Wait()` blocking on async code.
- Attributes are load-bearing in Eco mods (`[Serialized]`, `[LocDisplayName]`, `[Tier]`, etc.). Don't drop or reorder them casually.

## When this skill is active

Editing or writing C#. Inherit Kai's defaults before reaching for training-data conventions. For Eco gameplay specifics, pull in the `gaming-eco-*` skills.

## See also

- `kai-tech-prefs` - tooling and dependency preferences.
