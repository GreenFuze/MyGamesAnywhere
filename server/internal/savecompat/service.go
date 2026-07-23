package savecompat

import (
	"context"
	"errors"
	"fmt"
)

type Converter interface {
	Definition() ConverterDefinition
	Convert(context.Context, Snapshot) (Snapshot, error)
}

// AtomicSnapshotWriter replaces a destination only after it has durably staged
// and validated the candidate. On error it must preserve its previous snapshot.
type AtomicSnapshotWriter interface {
	Replace(context.Context, Snapshot) error
}

type Service struct {
	repository Repository
	converters map[string]Converter
}

func NewService(repository Repository, converters ...Converter) (*Service, error) {
	if repository == nil {
		return nil, errors.New("save compatibility repository is required")
	}
	service := &Service{repository: repository, converters: make(map[string]Converter, len(converters))}
	for _, converter := range converters {
		if converter == nil {
			return nil, errors.New("save converter is required")
		}
		definition := converter.Definition()
		if err := definition.Validate(); err != nil {
			return nil, fmt.Errorf("invalid save converter: %w", err)
		}
		key := converterKey(definition.ID, definition.Version)
		if service.converters[key] != nil {
			return nil, fmt.Errorf("duplicate save converter %s %s", definition.ID, definition.Version)
		}
		service.converters[key] = converter
	}
	return service, nil
}

func (s *Service) ConvertAndCommit(ctx context.Context, source Snapshot, target FormatRef, writer AtomicSnapshotWriter) error {
	if s == nil || s.repository == nil {
		return errors.New("save compatibility service is unavailable")
	}
	if writer == nil {
		return errors.New("atomic save snapshot writer is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("validate source save snapshot: %w", err)
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("validate target save format: %w", err)
	}
	rule, err := s.repository.FindRule(ctx, source.Format, target)
	if err != nil {
		return fmt.Errorf("find save compatibility: %w", err)
	}
	if rule == nil || !rule.Enabled {
		return errors.New("no enabled compatibility evidence exists for these exact save formats")
	}
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("validate save compatibility rule: %w", err)
	}

	var candidate Snapshot
	switch rule.Relationship {
	case RelationshipSameFormat:
		candidate = source.Clone()
		candidate.Format = target
	case RelationshipConverter:
		candidate, err = s.convert(ctx, *rule, source)
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported save compatibility relationship")
	}
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("validate converted save snapshot: %w", err)
	}
	if !candidate.Format.Equal(target) {
		return errors.New("save converter returned the wrong target format")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writer.Replace(ctx, candidate.Clone()); err != nil {
		return fmt.Errorf("atomically replace save snapshot: %w", err)
	}
	return nil
}

func (s *Service) convert(ctx context.Context, rule CompatibilityRule, source Snapshot) (candidate Snapshot, err error) {
	definition, err := s.repository.GetConverter(ctx, rule.ConverterID, rule.ConverterVersion)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load save converter: %w", err)
	}
	if definition == nil || !definition.Enabled {
		return Snapshot{}, errors.New("the required save converter is not enabled")
	}
	if err := definition.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("validate persisted save converter: %w", err)
	}
	if !definition.Source.Equal(rule.Source) || !definition.Target.Equal(rule.Target) {
		return Snapshot{}, errors.New("save converter formats do not match the compatibility rule")
	}
	if definition.Reversible != rule.Reversible {
		return Snapshot{}, errors.New("save converter reversibility does not match the compatibility rule")
	}
	converter := s.converters[converterKey(definition.ID, definition.Version)]
	if converter == nil {
		return Snapshot{}, errors.New("the required save converter code is not installed")
	}
	if !definition.ExecutableMatches(converter.Definition()) {
		return Snapshot{}, errors.New("installed save converter metadata does not match the persisted registry")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			candidate = Snapshot{}
			err = fmt.Errorf("save converter panicked: %v", recovered)
		}
	}()
	candidate, err = converter.Convert(ctx, source.Clone())
	if err != nil {
		return Snapshot{}, fmt.Errorf("convert save snapshot: %w", err)
	}
	return candidate.Clone(), nil
}

func converterKey(id, version string) string {
	return id + "\x00" + version
}
